// Package httpregistry puts the registry on the network: a Server that
// exposes any ports.Registry over plain HTTP/JSON, and a Client that
// implements ports.Registry against such a server. This is what lets
// separate PROCESSES share the v1 "single honest instance" — one daemon
// hosts it, everyone else dials it. The interface stays the seam: a
// chain-backed registry would replace the server, and no client would
// know.
//
// No TLS, no auth — same trusted-network stance as tcpnet, stated out
// loud.
package httpregistry

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// entryJSON is the wire form (hex strings, greppable), mirroring
// fileregistry's on-disk format.
type tokenSigJSON struct {
	Validator string `json:"validator"`
	Sig       string `json:"sig"` // base64
}

type tokenJSON struct {
	Serial string         `json:"serial"` // base64
	Sigs   []tokenSigJSON `json:"sigs"`
}

type entryJSON struct {
	Root           string     `json:"root"`
	ManifestChunks []string   `json:"manifest_chunks"`
	FileSize       int64      `json:"file_size"`
	Publisher      string     `json:"publisher,omitempty"`
	Token          *tokenJSON `json:"token,omitempty"`
}

func toJSON(e ports.Entry) entryJSON {
	ej := entryJSON{Root: e.Root.String(), FileSize: e.FileSize}
	for _, id := range e.ManifestChunks {
		ej.ManifestChunks = append(ej.ManifestChunks, id.String())
	}
	if e.Publisher != (ports.NodeID{}) {
		ej.Publisher = e.Publisher.String()
	}
	if e.Token != nil {
		tj := &tokenJSON{Serial: base64.StdEncoding.EncodeToString(e.Token.Serial)}
		for _, s := range e.Token.Sigs {
			tj.Sigs = append(tj.Sigs, tokenSigJSON{
				Validator: s.Validator.String(),
				Sig:       base64.StdEncoding.EncodeToString(s.Sig),
			})
		}
		ej.Token = tj
	}
	return ej
}

func fromJSON(ej entryJSON) (ports.Entry, error) {
	root, err := ports.ParseHash(ej.Root)
	if err != nil {
		return ports.Entry{}, err
	}
	e := ports.Entry{Root: root, FileSize: ej.FileSize}
	for _, s := range ej.ManifestChunks {
		id, err := ports.ParseHash(s)
		if err != nil {
			return ports.Entry{}, err
		}
		e.ManifestChunks = append(e.ManifestChunks, id)
	}
	if ej.Publisher != "" {
		if e.Publisher, err = ports.ParseHash(ej.Publisher); err != nil {
			return ports.Entry{}, err
		}
	}
	if ej.Token != nil {
		serial, err := base64.StdEncoding.DecodeString(ej.Token.Serial)
		if err != nil {
			return ports.Entry{}, err
		}
		tok := &ports.PublishToken{Serial: serial}
		for _, s := range ej.Token.Sigs {
			v, err := ports.ParseHash(s.Validator)
			if err != nil {
				return ports.Entry{}, err
			}
			sig, err := base64.StdEncoding.DecodeString(s.Sig)
			if err != nil {
				return ports.Entry{}, err
			}
			tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: v, Sig: sig})
		}
		e.Token = tok
	}
	return e, nil
}

// Serve exposes reg on addr over plain HTTP (tests, trusted loopback).
func Serve(addr string, reg ports.Registry) (boundAddr string, shutdown func(), err error) {
	return serve(addr, reg, nil)
}

// ServeTLS exposes reg over TLS with the daemon's identity certificate;
// clients pin the daemon's NodeID (NewPinnedClient) — same
// key-is-identity scheme as tcpnet, no CA anywhere.
func ServeTLS(addr string, ident *identity.Identity, reg ports.Registry) (boundAddr string, shutdown func(), err error) {
	cert, err := ident.Certificate()
	if err != nil {
		return "", nil, err
	}
	return serve(addr, reg, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})
}

// asyncPublisher is an optional Registry capability (chainhost.Host implements it; a plain
// fileregistry does not). PublishAsync validates SYNCHRONOUSLY — so refusals (token /
// durable-Publisher / double-spend / dup) surface at once — but commits ASYNC: the handler
// replies 202 and the client polls PublishStatus until the entry commits OR the gather
// reaches a terminal failure. This removes the flat 10s-client / 30s-server deadlines that
// guillotined a ~1.5 MB genesis gather (#286 Layer 1) WITHOUT holding a connection open for
// the whole gather (no slowloris, #48). PublishStatus is what lets the client fast-fail on a
// no-quorum round — so a publish-retry-until-standing caller retries promptly — instead of
// polling out the full budget. A registry that commits instantly (fileregistry) implements
// neither, so the sync Publish is used.
type asyncPublisher interface {
	PublishAsync(context.Context, ports.Entry) error
	// PublishStatus reports (committed, failErr): committed=true when the entry is on the
	// chain; failErr!=nil when the gather terminally failed this round; (false, nil) = pending.
	PublishStatus(context.Context, ports.Hash) (bool, error)
}

// publishStatusJSON is the wire form of an async publish's terminal state.
type publishStatusJSON struct {
	State string `json:"state"` // "committed" | "failed" | "pending"
	Error string `json:"error,omitempty"`
}

const (
	publishPollInterval = 1 * time.Second
	// Generous accept→commit budget: a first quorum-2 genesis block carries the proposer's
	// ~1.5 MB bond registration whose gather over a 3-region WAN can take tens of seconds.
	publishPollTimeout = 180 * time.Second
)

func serve(addr string, reg ports.Registry, tlsCfg *tls.Config) (boundAddr string, shutdown func(), err error) {
	// Read-cost bounding (#48): a per-IP rate limit + server timeouts keep a public
	// registry cheap to run and hard to exhaust (slowloris, lookup floods).
	lim := newIPRateLimiter(defaultRatePerSec, defaultBurst)
	mux := http.NewServeMux()

	mux.HandleFunc("POST /publish", func(w http.ResponseWriter, r *http.Request) {
		var ej entryJSON
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&ej); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e, err := fromJSON(ej)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Async path (chainhost): validate synchronously (refusals below map to their
		// status codes), then 202 Accepted — the commit gather runs in the background and
		// the client polls Lookup. A sync registry (fileregistry) falls through to Publish.
		if ap, ok := reg.(asyncPublisher); ok {
			switch err := ap.PublishAsync(r.Context(), e); {
			case err == nil:
				w.WriteHeader(http.StatusAccepted) // 202 — accepted; poll /lookup for commit
			case errors.Is(err, ports.ErrDupPublish):
				http.Error(w, err.Error(), http.StatusConflict)
			case errors.Is(err, ports.ErrInsufficientCredit):
				http.Error(w, err.Error(), http.StatusPaymentRequired)
			case errors.Is(err, ports.ErrPublisherRequired):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		switch err := reg.Publish(r.Context(), e); {
		case err == nil:
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, ports.ErrDupPublish):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, ports.ErrInsufficientCredit):
			http.Error(w, err.Error(), http.StatusPaymentRequired)
		case errors.Is(err, ports.ErrPublisherRequired):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("GET /lookup", func(w http.ResponseWriter, r *http.Request) {
		root, err := ports.ParseHash(r.URL.Query().Get("root"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e, ok, err := reg.Lookup(r.Context(), root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(toJSON(e))
	})

	// GET /publish-status?root= reports the terminal state of an async publish (202) so the
	// client stops polling the instant the gather resolves either way — committed or a
	// terminal no-quorum failure — instead of waiting out the accept→commit budget. A sync
	// registry (fileregistry) commits inline and serves no status: 404, and its clients never
	// hit the 202 path anyway.
	mux.HandleFunc("GET /publish-status", func(w http.ResponseWriter, r *http.Request) {
		ap, ok := reg.(asyncPublisher)
		if !ok {
			http.NotFound(w, r)
			return
		}
		root, err := ports.ParseHash(r.URL.Query().Get("root"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		committed, failErr := ap.PublishStatus(r.Context(), root)
		st := publishStatusJSON{State: "pending"}
		switch {
		case committed:
			st.State = "committed"
		case failErr != nil:
			st.State = "failed"
			st.Error = failErr.Error()
		}
		json.NewEncoder(w).Encode(st)
	})

	// GET /all is DELIBERATELY NOT SERVED on the public mux (red-team blind-2026-08-08
	// F-3). It serialized the whole registry O(N) with no pagination, an unbounded
	// per-request cost that an interim work-pricing bounded only per-SOURCE — a
	// distributed dump (one request per source IP) no per-IP counter can touch. It is
	// used only by an operator's OWN CLI/UI, which reads the registry IN-PROCESS (the
	// daemon's local chainhost/fileregistry), never over this wire. Removing it deletes
	// the amplification and the distributed variant outright. A client asking for /all
	// over HTTP now gets 404 by design; the operator UI degrades gracefully. If a public
	// bulk-read is ever wanted, add it back paginated (cursor + hard page cap) + priced.

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	// Server timeouts round out the read-cost bounding (#48) against slowloris and slow
	// reads/writes (the per-IP limiter `lim` is created above; /all is priced by work).
	srv := &http.Server{
		Handler:           lim.limit(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go srv.Serve(ln)
	return ln.Addr().String(), func() { lim.close(); srv.Close() }, nil
}

// Client implements ports.Registry against a Serve endpoint.
type Client struct {
	base string
	hc   *http.Client
}

var _ ports.Registry = (*Client)(nil)

func NewClient(baseURL string) *Client {
	return &Client{base: baseURL, hc: &http.Client{Timeout: 10 * time.Second}}
}

// NewPinnedClient talks https to a registry hosted by the daemon whose
// NodeID is expect; any other key at that address fails the handshake.
func NewPinnedClient(baseURL string, expect ports.NodeID) *Client {
	return &Client{base: baseURL, hc: &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify:    true, // replaced by pinning, not skipped
			VerifyPeerCertificate: identity.VerifyExpected(expect),
			MinVersion:            tls.VersionTLS13,
		}},
	}}
}

func (c *Client) Publish(ctx context.Context, e ports.Entry) error {
	body, err := json.Marshal(toJSON(e))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/publish", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("httpregistry publish: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusAccepted:
		// Async accept (#286 Layer 1): the commit gather runs on the server; poll
		// /publish-status until it resolves — committed, or a terminal no-quorum failure —
		// or the budget elapses. Each poll is a fast, fresh request (no held connection → no
		// flat 10s guillotine on the commit, no slowloris). Publish still BLOCKS the caller
		// until committed, so sequencing (e.g. a double-spend replay seeing the first token
		// spent) is preserved exactly as the sync path. Surfacing the terminal failure is
		// what lets a publish-retry-until-standing caller retry promptly rather than poll out
		// the whole budget on a round that will never commit.
		deadline := time.Now().Add(publishPollTimeout)
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
		for {
			committed, failMsg, serr := c.publishStatus(ctx, e.Root)
			if serr == nil {
				if committed {
					return nil
				}
				if failMsg != "" {
					return fmt.Errorf("httpregistry publish: %s", failMsg)
				}
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("httpregistry publish: accepted but not committed within %s (root %s) — the consensus gather did not finish; see the validators' -log debug", publishPollTimeout, e.Root)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(publishPollInterval):
			}
		}
	case http.StatusConflict:
		return ports.ErrDupPublish
	case http.StatusPaymentRequired:
		return ports.ErrInsufficientCredit
	default:
		msg := bytes.TrimSpace(mustRead(resp.Body))
		// The classic first-run mistake: http:// against a pinned HTTPS
		// registry. The server says so; translate it into the fix.
		if bytes.Contains(msg, []byte("HTTP request to an HTTPS server")) {
			return fmt.Errorf("httpregistry publish: this registry is key-pinned HTTPS — use the ID@https://host:port ref the daemon prints, not http://")
		}
		return fmt.Errorf("httpregistry publish: %s: %s", resp.Status, msg)
	}
}

func mustRead(r io.Reader) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, 4096))
	return b
}

const (
	// getRetries / getRetryBackoff: a bounded, exponential-backoff retry for the client's
	// idempotent GET reads. This is the one client path NOT behind the consensus layer's
	// retry, so a single dropped packet used to fail a swarm-get / root resolution outright
	// — the durable-WAN discipline is to ride out transient loss, not decide on one sample
	// (immutable #5; docs/network-durability.md §1 "modest initial + retry", §2). GET only:
	// the request has no body, so it is safely reissued. POST /publish is NOT retried here
	// (it has side effects; its commit durability comes from the async 202 + poll instead).
	getRetries      = 2 // 3 attempts total
	getRetryBackoff = 200 * time.Millisecond
)

// doGetRetry issues an idempotent GET with bounded exponential backoff on TRANSIENT
// failures (a network error or a 5xx). A definitive response (2xx/3xx/4xx) returns at
// once — a 404 or other 4xx is an answer, not a blip. Respects the request context.
func (c *Client) doGetRetry(req *http.Request) (*http.Response, error) {
	backoff := getRetryBackoff
	var lastErr error
	for attempt := 0; ; attempt++ {
		resp, err := c.hc.Do(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s", resp.Status)
		}
		if attempt >= getRetries {
			return nil, lastErr
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

func (c *Client) Lookup(ctx context.Context, root ports.Hash) (ports.Entry, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/lookup?root="+root.String(), nil)
	if err != nil {
		return ports.Entry{}, false, err
	}
	resp, err := c.doGetRetry(req)
	if err != nil {
		return ports.Entry{}, false, fmt.Errorf("httpregistry lookup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ports.Entry{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ports.Entry{}, false, fmt.Errorf("httpregistry lookup: %s", resp.Status)
	}
	var ej entryJSON
	if err := json.NewDecoder(resp.Body).Decode(&ej); err != nil {
		return ports.Entry{}, false, err
	}
	e, err := fromJSON(ej)
	return e, err == nil, err
}

// publishStatus polls an async publish's terminal state (see the /publish-status handler).
// Returns (committed, failMsg, err): committed=true once the entry is on the chain; a
// non-empty failMsg is a terminal gather failure to surface now; both zero = still pending.
// A registry without the async status endpoint answers 404 → treated as pending so the
// caller keeps polling (it will hit its budget only if the commit truly never lands).
func (c *Client) publishStatus(ctx context.Context, root ports.Hash) (committed bool, failMsg string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/publish-status?root="+root.String(), nil)
	if err != nil {
		return false, "", err
	}
	resp, err := c.doGetRetry(req)
	if err != nil {
		return false, "", fmt.Errorf("httpregistry publish-status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, "", nil // no async status endpoint — treat as pending
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("httpregistry publish-status: %s", resp.Status)
	}
	var st publishStatusJSON
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return false, "", err
	}
	return st.State == "committed", st.Error, nil
}

// ErrAllNotServed is returned by a remote client's All(): a registry no longer
// serves a bulk /all dump on its public mux (F-3). A caller should degrade (list
// nothing / use /lookup), not treat it as a hard error. An operator listing its OWN
// registry reads it in-process, not through this client, so is unaffected.
var ErrAllNotServed = errors.New("httpregistry: /all is not served on the public registry mux (use per-root lookup)")

func (c *Client) All(ctx context.Context) ([]ports.Entry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/all", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doGetRetry(req)
	if err != nil {
		return nil, fmt.Errorf("httpregistry all: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAllNotServed
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("httpregistry all: %s", resp.Status)
	}
	var ejs []entryJSON
	if err := json.NewDecoder(resp.Body).Decode(&ejs); err != nil {
		return nil, err
	}
	entries := make([]ports.Entry, 0, len(ejs))
	for _, ej := range ejs {
		e, err := fromJSON(ej)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
