package e2e

// R2.9 — the paid DELIVERY SESSION, end to end over real TCP (in-process loops, the
// TestPaidRelaySessionEndToEnd shape): a durable fetcher pins the server's chain-
// committed key, buys ONE demand token over the wire (the real blind withdrawal,
// charged by ChargePublish on the SERVER's ledger), opens a session with it
// (MsgDeliveryOpen: the token spent into the guard at OPEN), settles a cumulative-count
// receipt for an object (MsgDeliverySettle, receipt v3), and the session is closed on
// idleness with its remainder accounted once. Asserts: (a) the settlement pays exactly
// count·p − skim into the server's balance; (b) the ledger total moves by settled − face
// over withdrawal → open → settle (≤ 0; the remainder is burned under G-6 as ratified);
// (c) the Invariant-A firewall (Reputation unmoved); (d) the S5 markers "delivery
// receipt banked" and "delivery session closed" are emitted; (e) the close line carries
// no identity and no object (the M0 log audit). Gate G-R212-8 §8 e2e; B-11's line is
// the daemon's (TestAffordabilityLineIsAnnounced).

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestPaidDeliverySessionEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e drives real TCP + loops; skipped under -short")
	}
	const fee = int64(50_000)
	const count = uint64(16) // 16 × 256 KiB acknowledged = 4 MiB of one object
	const idle = 2 * ports.Second

	dLoop, sLoop := eventloop.New(), eventloop.New()
	go dLoop.Run()
	go sLoop.Run()
	t.Cleanup(dLoop.Stop)
	t.Cleanup(sLoop.Stop)

	dID := identity.FromSeed(9300) // the fetcher's DURABLE identity — buys the anchor AND opens the session
	sID := identity.FromSeed(9301) // the server: issuer == server (the bilateral shape)

	dTr, err := tcpnet.New(dLoop, dID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dTr.Close() })
	sTr, err := tcpnet.New(sLoop, sID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sTr.Close() })

	dNode := node.New(dID.NodeID(), node.DefaultConfig(), walltime.New(dLoop), dTr, memstore.New())
	sNode := node.New(sID.NodeID(), node.DefaultConfig(), walltime.New(sLoop), sTr, memstore.New())

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sNode.SetSigner(sID.Signer())
	sNode.EnableChain(anchorChainFor(t, sID, serverKey), sID.Signer())
	sNode.SetDemandIssuerKey(rand.Reader, 0, serverKey)
	dNode.SetSigner(dID.Signer())
	dNode.EnableChain(anchorChainFor(t, sID, serverKey), dID.Signer())

	ledger := credit.New(fee, 500_000)
	ledger.Register(sID.NodeID())
	ledger.Register(dID.NodeID())
	sNode.SetLedger(ledger)
	log := &capLog{}
	sNode.SetLogger(log)
	sNode.EnableDemandBank(sNode.ID())
	sNode.EnableDeliverySessions(idle)

	dTr.AddPeer(sID.NodeID(), sTr.Addr())
	sTr.AddPeer(dID.NodeID(), dTr.Addr())

	readBalance := func(id ports.NodeID) int64 {
		ch := make(chan int64, 1)
		sLoop.Post("check-balance", func() { ch <- ledger.Balance(id) })
		return <-ch
	}
	object := ports.HashBytes([]byte("paid-delivery-e2e-object"))
	// Σ_L = Σ balances + Σ escrow: the settlement's skim lands in the OBJECT's escrow
	// (unlike the relay lane, which has no skim), so the conservation oracle counts it.
	readTotal := func() int64 {
		ch := make(chan int64, 1)
		sLoop.Post("check-total", func() {
			var sum int64
			for _, b := range ledger.Balances() {
				sum += b
			}
			ch <- sum + ledger.EscrowBalance(object)
		})
		return <-ch
	}
	readReputation := func() int64 {
		ch := make(chan int64, 1)
		sLoop.Post("check-rep", func() { ch <- ledger.Reputation(sID.NodeID()) })
		return <-ch
	}
	readStats := func() credit.DeliverySettlementStats {
		ch := make(chan credit.DeliverySettlementStats, 1)
		sLoop.Post("check-stats", func() { ch <- ledger.DeliverySettlementStats() })
		return <-ch
	}
	totalStart, dStart, sStart, repStart := readTotal(), readBalance(dID.NodeID()), readBalance(sID.NodeID()), readReputation()

	// ---- Pin the server's committed key_0 and buy ONE demand token over the wire.
	pinned := make(chan error, 1)
	dLoop.Post("pin", func() {
		dNode.FetchDemandIssuerKeys(sID.NodeID(), func(n int, e error) {
			if e == nil && n != 1 {
				e = fmt.Errorf("pinned %d keys, want 1", n)
			}
			pinned <- e
		})
	})
	select {
	case e := <-pinned:
		if e != nil {
			t.Fatalf("pin: %v", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pin timed out")
	}
	tokCh, tokErr := make(chan demand.Token, 1), make(chan error, 1)
	dLoop.Post("withdraw", func() {
		dNode.AcquireDemandTokenInWindow(rand.Reader, sID.NodeID(), func(tk demand.Token, _ uint64, e error) {
			if e != nil {
				tokErr <- e
				return
			}
			tokCh <- tk
		})
	})
	var tok demand.Token
	select {
	case tok = <-tokCh:
	case e := <-tokErr:
		t.Fatalf("withdraw: %v", e)
	case <-time.After(10 * time.Second):
		t.Fatal("withdraw timed out")
	}
	if got := readBalance(dID.NodeID()); got != dStart-fee {
		t.Fatalf("buyer balance %d after one withdrawal, want %d − fee", got, dStart)
	}

	// ---- Open the session over the wire: the token is spent at OPEN.
	handleCh, mCh, openErr := make(chan uint64, 1), make(chan []byte, 1), make(chan error, 1)
	dLoop.Post("open", func() {
		dNode.OpenDeliverySessionRemote(sID.NodeID(), []demand.Token{tok}, func(h uint64, m []byte, e error) {
			if e != nil {
				openErr <- e
				return
			}
			handleCh <- h
			mCh <- m
		})
	})
	var handle uint64
	var commitment []byte
	select {
	case handle = <-handleCh:
		commitment = <-mCh
	case e := <-openErr:
		t.Fatalf("open: %v", e)
	case <-time.After(10 * time.Second):
		t.Fatal("open timed out")
	}
	if handle == 0 {
		t.Fatal("zero handle")
	}

	// ---- Settle a cumulative-count receipt (v3) for one object.
	settledCh, settleErr := make(chan int64, 1), make(chan error, 1)
	dLoop.Post("settle", func() {
		dNode.SubmitDeliverySettle(sID.NodeID(), handle, commitment, object, count, func(s int64, e error) {
			if e != nil {
				settleErr <- e
				return
			}
			settledCh <- s
		})
	})
	var settled int64
	select {
	case settled = <-settledCh:
	case e := <-settleErr:
		t.Fatalf("settle: %v", e)
	case <-time.After(10 * time.Second):
		t.Fatal("settle timed out")
	}
	value := int64(count) * credit.DeliveryIncrementCredit
	if settled != value {
		t.Fatalf("settled %d, want count·p = %d", settled, value)
	}
	// (a) the conserved payout: count·p − skim into the server's balance.
	wantCredit := value - value*credit.SkimNum/credit.SkimDen
	if bal := readBalance(sID.NodeID()); bal != sStart+wantCredit {
		t.Fatalf("server balance %d, want %d + %d", bal, sStart, wantCredit)
	}
	// (b) Δ Σ_L = settled − face ≤ 0 across withdrawal → open → settle.
	if delta := readTotal() - totalStart; delta != value-fee || delta > 0 {
		t.Fatalf("ledger total moved by %d, want settled − face = %d ≤ 0", delta, value-fee)
	}
	// (c) the Invariant-A firewall.
	if rep := readReputation(); rep != repStart {
		t.Fatalf("Reputation moved %d → %d on a delivery settlement", repStart, rep)
	}
	// (d) the S5 marker.
	find := func(marker string) string {
		for _, ln := range log.lines() {
			if strings.Contains(ln, marker) {
				return ln
			}
		}
		return ""
	}
	if find("delivery receipt banked") == "" {
		t.Fatal("no 'delivery receipt banked' line for a settled session receipt")
	}

	// ---- Idle close: after the window, the sweep closes the session and accounts the
	// remainder ONCE (burned under G-6 as ratified). The daemon drives this sweep on a
	// ticker; here it is posted onto the server's loop after the window elapsed.
	time.Sleep(time.Duration(idle) + 500*time.Millisecond)
	sLoop.Post("sweep", sNode.SweepDeliverySessions)
	deadline := time.Now().Add(5 * time.Second)
	for find("delivery session closed") == "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	closed := find("delivery session closed")
	if closed == "" {
		t.Fatal("the idle session was not closed")
	}
	if !strings.Contains(closed, "reason=idle") && !strings.Contains(closed, "idle") {
		t.Fatalf("close reason: %q", closed)
	}
	st := readStats()
	if st.SessionsClosed != 1 || st.PendingRefundCredits != fee-value || st.BurnedCredits != 0 || st.SettledCredits != value {
		t.Fatalf("stats %+v, want 1 closed, face − settled = %d booked as a deposit, 0 burned, settled %d", st, fee-value, value)
	}
	if delta := readTotal() - totalStart; delta != value-fee {
		t.Fatalf("the close moved the ledger total (now %d) — the remainder is a deposit until the anchor expires, not routed", delta)
	}
	// (e) the M0 log audit: the close line names no identity and no object.
	for _, forbidden := range []string{dID.NodeID().String(), sID.NodeID().String(), object.String(), hex.EncodeToString(tok.Serial)} {
		if forbidden != "" && strings.Contains(closed, forbidden) {
			t.Fatalf("close line leaks %q: %q", forbidden, closed)
		}
	}
}

// TestDeliveryIdleWindowFloorIsEnforcedAtStartUp — the runtime half of the idle-window
// floor. Refuse-until-set is RELEASED (Lane C2, owner call 4 of D-TRUE-UP-CALLS-2026-09-07:
// the default ships now that the bound is field-confirmed), so what the daemon enforces is
// no longer "set it" but "set it high enough": a window below deliveryIdleFloor reaps an
// honest fetcher gapped by a stall the liveness model admits, and is refused.
//
// Both polarities, at the endpoints of the floor rather than at a token value: one second
// UNDER the derived floor refuses, and the shipped DEFAULT (no flag at all) boots and
// announces the window the reaper actually installed.
// The old "unset refuses" arm is gone because the behaviour it asserted is what shipped.
func TestDeliveryIdleWindowFloorIsEnforcedAtStartUp(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	for _, arm := range []struct {
		name, seed string
		extra      []string
	}{
		// 1ns: the original floor's job (item 3 — the idle/2 ticker must never see a
		// zero interval). Still refused, now by the bound-derived floor above it.
		{"sub-second", "4831", []string{"-delivery-idle-window", "1ns"}},
		// Under the derived floor of 9m33.333333333s = bound × 4/3, by the smallest
		// amount the flag can be written in whole seconds (1.333 s). This is the arm that
		// fails if the floor is ever quietly lowered back toward the bound itself: 9m32s
		// clears 430 s naively and is still refused, because the stamp coarsening spends
		// a quarter of it.
		{"just-under-the-derived-floor", "4832", []string{"-delivery-idle-window", "9m32s"}},
	} {
		arm := arm
		t.Run(arm.name, func(t *testing.T) {
			args := append([]string{
				"-listen", "127.0.0.1:0", "-store", t.TempDir(),
				"-serve-registry", "127.0.0.1:0", "-validator",
				"-accept-delivery-receipts", "-epoch-blocks", "8",
				"-grant-capacity", "256", "-grant-per-hour", "256",
				"-objective=false", "-min-rep", "100", "-quorum", "1",
				"-bond", "8M", "-min-bond-floor", "0",
				"-capacity", "1G", "-mdns=false", "-id-seed", arm.seed}, arm.extra...)
			d := startDaemon(t, "r29-idle-"+arm.name, args...)
			line := d.waitFor(t, reRefuseLine, 20*time.Second)[0]
			if !strings.Contains(line, "-delivery-idle-window") {
				t.Fatalf("the refusal does not name -delivery-idle-window:\n\t%s", line)
			}
			done := make(chan error, 1)
			go func() { done <- d.cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("the daemon exited 0 after a refusal\n--- output ---\n%s", d.out.dump())
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("the daemon printed the refusal but did not exit\n--- output ---\n%s", d.out.dump())
			}
			if m := d.out.find(rePeer); m != nil {
				t.Fatalf("a refused daemon became a peer first: %q", m[0])
			}
		})
	}
}

// TestDeliveryIdleWindowDefaultBootsThePaidLane — the OTHER polarity of the gate above,
// and the runtime arm the cmd/silt source pin cannot reach: a daemon that arms the paid
// delivery lane and sets NO -delivery-idle-window boots, announces the lane, and echoes
// the shipped default on its affordability line. Before Lane C2 this exact argv refused.
//
// It asserts the ANNOUNCED window, not just the absence of a refusal, because that is the
// only surface an operator reads to learn what window it got.
func TestDeliveryIdleWindowDefaultBootsThePaidLane(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "r29-idle-default",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-accept-delivery-receipts", "-epoch-blocks", "8",
		"-grant-capacity", "256", "-grant-per-hour", "256",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4833")
	line := d.waitFor(t, regexp.MustCompile(`delivery settlement: .*idle window \S+`), 30*time.Second)[0]
	if !strings.Contains(line, "idle window 24m0s") {
		t.Fatalf("the daemon booted on an idle window it did not announce as the shipped default:\n\t%s", line)
	}
	if m := d.out.find(reRefuseLine); m != nil {
		t.Fatalf("the default window was refused at start-up: %q", m[0])
	}
}
