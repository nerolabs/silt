package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/nerolabs/silt/core/advisory"
	"github.com/nerolabs/silt/core/release"
	"github.com/nerolabs/silt/ports"
)

// parseMaintainers reads -maintainers and -maintainer-threshold. It refuses a key set without a
// threshold of at least 2, a threshold above the key count, a threshold without keys, and a
// repeated key: each is a configuration under which "no single key may declare a floor" would not
// hold, or would hold only by accident.
func parseMaintainers(list string, threshold int) ([][]byte, error) {
	list = strings.TrimSpace(list)
	if list == "" {
		if threshold != 0 {
			return nil, errors.New("-maintainer-threshold is set but -maintainers is empty")
		}
		return nil, nil
	}
	seen := map[string]bool{}
	var keys [][]byte
	for _, h := range strings.Split(list, ",") {
		k, err := hex.DecodeString(strings.TrimSpace(h))
		if err != nil || len(k) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("-maintainers: %q is not a hex ed25519 public key", h)
		}
		if seen[string(k)] {
			return nil, fmt.Errorf("-maintainers: key %s is listed twice", h)
		}
		seen[string(k)] = true
		keys = append(keys, k)
	}
	if threshold < 2 || threshold > len(keys) {
		return nil, fmt.Errorf("-maintainer-threshold must be between 2 and %d (the number of -maintainers), got %d: no single key may declare a version floor", len(keys), threshold)
	}
	return advisory.SortKeys(keys), nil
}

const advisoryUsage = `silt advisory — the maintainers' version-floor advisory (TENETS R4)

  silt advisory keygen -out FILE
      make a maintainer key; prints its public key for the network's -maintainers
  silt advisory sign -key FILE -chain-id HEX -file ADV [-seq N -floor R -tier T -grace B -note TEXT]
      add this maintainer's signature to ADV, creating it from the flags if it does not exist.
      T is notice, urgent, enforce or recall. A recall names no floor.
  silt advisory show -file ADV
  silt advisory submit -file ADV -api http://127.0.0.1:PORT -token TOKEN
      hand the signed advisory to a validator's daemon; it commits once a block carries it

This build is release %d.
`

func cmdAdvisory(args []string) error {
	if len(args) == 0 {
		fmt.Printf(advisoryUsage, release.Number)
		return nil
	}
	switch args[0] {
	case "keygen":
		return advisoryKeygen(args[1:])
	case "sign":
		return advisorySign(args[1:])
	case "show":
		return advisoryShow(args[1:])
	case "submit":
		return advisorySubmit(args[1:])
	}
	return fmt.Errorf("unknown advisory command %q\n\n"+advisoryUsage, args[0], release.Number)
}

func advisoryKeygen(args []string) error {
	fs := flag.NewFlagSet("advisory keygen", flag.ContinueOnError)
	out := fs.String("out", "", "write the maintainer key here (mode 0600); it must not exist")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("advisory keygen: -out is required")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, hex.EncodeToString(priv.Seed())); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("maintainer public key: %s\n", hex.EncodeToString(pub))
	return nil
}

func readMaintainerKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a maintainer key (want a hex ed25519 seed)", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func parseTier(s string) (advisory.Tier, error) {
	switch s {
	case "recall":
		return advisory.TierRecall, nil
	case "notice":
		return advisory.TierNotice, nil
	case "urgent":
		return advisory.TierUrgent, nil
	case "enforce":
		return advisory.TierEnforce, nil
	}
	return 0, fmt.Errorf("-tier must be notice, urgent, enforce or recall, got %q", s)
}

func advisorySign(args []string) error {
	fs := flag.NewFlagSet("advisory sign", flag.ContinueOnError)
	keyPath := fs.String("key", "", "this maintainer's key file")
	chainHex := fs.String("chain-id", "", "the network's chain id (its genesis hash, hex), so the signature binds to this network only")
	file := fs.String("file", "", "the advisory file: signed in place, created from the flags below if absent")
	seq := fs.Uint64("seq", 0, "sequence: above the advisory in force")
	floor := fs.Uint("floor", 0, "the lowest release validators are asked to run")
	tier := fs.String("tier", "", "notice, urgent, enforce or recall")
	grace := fs.Uint64("grace", 0, "blocks after commit before the enforce tier takes effect")
	note := fs.String("note", "", "what the release fixes and where to get it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyPath == "" || *chainHex == "" || *file == "" {
		return errors.New("advisory sign: -key, -chain-id and -file are required")
	}
	idRaw, err := hex.DecodeString(*chainHex)
	if err != nil || len(idRaw) != len(ports.Hash{}) {
		return fmt.Errorf("-chain-id: want %d hex bytes", len(ports.Hash{}))
	}
	var chainID ports.Hash
	copy(chainID[:], idRaw)
	priv, err := readMaintainerKey(*keyPath)
	if err != nil {
		return err
	}

	var s advisory.Signed
	if raw, rerr := os.ReadFile(*file); rerr == nil {
		if s, err = advisory.Unmarshal(raw); err != nil {
			return fmt.Errorf("%s: %v", *file, err)
		}
		fromFlags := false
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "seq", "floor", "tier", "grace", "note":
				fromFlags = true
			}
		})
		if fromFlags {
			return fmt.Errorf("%s exists: sign it as it is (its content is what the other maintainers signed), without -seq/-floor/-tier/-grace/-note", *file)
		}
	} else if errors.Is(rerr, os.ErrNotExist) {
		t, terr := parseTier(*tier)
		if terr != nil {
			return terr
		}
		s.Advisory = advisory.Advisory{Seq: *seq, Floor: uint32(*floor), Tier: t, GraceBlocks: *grace, Note: *note}
	} else {
		return rerr
	}
	if err := s.Advisory.Validate(); err != nil {
		return err
	}
	pub := priv.Public().(ed25519.PublicKey)
	for _, sig := range s.Signatures {
		if string(sig.Pub) == string(pub) {
			return fmt.Errorf("this key (%x) has already signed %s", pub, *file)
		}
	}
	if s, err = advisory.Sign(s, priv, chainID); err != nil {
		return err
	}
	raw, err := s.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(*file, raw, 0o644); err != nil {
		return err
	}
	fmt.Printf("signed: %s now carries %d signature(s)\n", *file, len(s.Signatures))
	return printAdvisory(s)
}

func advisoryShow(args []string) error {
	fs := flag.NewFlagSet("advisory show", flag.ContinueOnError)
	file := fs.String("file", "", "the advisory file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	s, err := advisory.Unmarshal(raw)
	if err != nil {
		return err
	}
	return printAdvisory(s)
}

func printAdvisory(s advisory.Signed) error {
	a := s.Advisory
	fmt.Printf("  seq %d, floor release %d, tier %d, grace %d block(s)\n  note: %q\n", a.Seq, a.Floor, a.Tier, a.GraceBlocks, a.Note)
	for _, sig := range s.Signatures {
		fmt.Printf("  signed by %x\n", sig.Pub)
	}
	return nil
}

func advisorySubmit(args []string) error {
	fs := flag.NewFlagSet("advisory submit", flag.ContinueOnError)
	file := fs.String("file", "", "the signed advisory file")
	api := fs.String("api", "", "a validator daemon's -ui address, e.g. http://127.0.0.1:8081")
	token := fs.String("token", "", "that daemon's API token (<store>/ui-token)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *api == "" || *token == "" {
		return errors.New("advisory submit: -file, -api and -token are required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if _, err := advisory.Unmarshal(raw); err != nil {
		return fmt.Errorf("%s: %v", *file, err)
	}
	form := url.Values{"advisory": {hex.EncodeToString(raw)}}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*api, "/")+"/api/advisory", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+*token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the daemon refused the advisory: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	fmt.Printf("submitted: %s\n", strings.TrimSpace(string(body)))
	return nil
}
