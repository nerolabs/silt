package credit

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R2.12 — the seam gates. A CONFIGURED ledger hands out a zero balance at Register and
// applies the starter grant at the first ADMITTED SPEND (CanPublish / ChargePublish /
// FundEscrow); a denial is retryable, never permanent; the owner is unmetered; the
// non-spend paths consume no token; the deny floor degrades instead of denying when set.
// The UNCONFIGURED ledger is byte-for-byte the pre-R2.12 faucet — every other test file in
// this package is that gate.

func r212Ledger(t *testing.T, capacity, refill, interval, floor int64) (*Ledger, *int64) {
	t.Helper()
	now := int64(0)
	l := New(50_000, 500_000)
	l.SetFaucet(capacity, refill, interval, floor, func() int64 { return now })
	if l.faucet == nil {
		t.Fatalf("fixture: faucet not configured")
	}
	return l, &now
}

func r212ID(i byte) ports.NodeID { return ports.HashBytes([]byte{i, 0x12}) }

// TestR212RegisterIsZeroAndTheGrantLandsAtTheFirstSpend: under a configured faucet a fresh
// identity is registered PENDING with a zero balance; its grant lands when it first reaches
// a spend gate, once; a second spend costs no token.
func TestR212RegisterIsZeroAndTheGrantLandsAtTheFirstSpend(t *testing.T) {
	l, _ := r212Ledger(t, 10, 10, 1_000, 0)
	a := r212ID(1)
	l.Register(a)
	if got := l.accounts[a].balance; got != 0 || !l.accounts[a].grantPending {
		t.Fatalf("configured Register: balance %d pending %v, want 0 / pending", got, l.accounts[a].grantPending)
	}
	if st := l.FaucetStats(); st.GrantsPending != 1 || st.Level != 10 {
		t.Fatalf("after Register: pending %d level %d, want 1 / 10 (Register consumes no token)", st.GrantsPending, st.Level)
	}
	if !l.CanPublish(a) {
		t.Fatalf("CanPublish refused a pending identity with a full bucket — the grant must land at the spend gate")
	}
	if got := l.Balance(a); got != 500_000 {
		t.Fatalf("balance after the first spend gate = %d, want the 500,000 grant", got)
	}
	st := l.FaucetStats()
	if st.GrantsIssued != 1 || st.GrantsPending != 0 || st.Level != 9 {
		t.Fatalf("stats after grant: %+v", st)
	}
	if err := l.ChargePublish(a); err != nil {
		t.Fatal(err)
	}
	if st := l.FaucetStats(); st.Level != 9 || st.GrantsIssued != 1 {
		t.Fatalf("a spend by an already-granted identity consumed a token: %+v", st)
	}
	// Each of the three gates grants a pending identity.
	b, c := r212ID(2), r212ID(3)
	if err := l.ChargePublish(b); err != nil || l.Balance(b) != 450_000 {
		t.Fatalf("ChargePublish did not grant-then-charge a pending identity: err %v balance %d", err, l.Balance(b))
	}
	if err := l.FundEscrow(ports.HashBytes([]byte("root")), c, 1_000); err != nil || l.Balance(c) != 499_000 {
		t.Fatalf("FundEscrow did not grant-then-fund a pending identity: err %v balance %d", err, l.Balance(c))
	}
}

// TestR212DenialIsPendingNotPermanent: with the bucket empty a fresh identity is refused at
// the spend gate and STAYS pending; when tokens accrue its next spend is granted. Nothing
// about the identity is remembered against it.
func TestR212DenialIsPendingNotPermanent(t *testing.T) {
	l, now := r212Ledger(t, 2, 2, 1_000, 0) // 2 per 1,000 ns
	l.CanPublish(r212ID(1))
	l.CanPublish(r212ID(2)) // bucket empty
	late := r212ID(3)
	if l.CanPublish(late) {
		t.Fatalf("empty bucket admitted a third identity")
	}
	if err := l.ChargePublish(late); err != ports.ErrInsufficientCredit {
		t.Fatalf("ChargePublish on a denied identity: %v, want ErrInsufficientCredit", err)
	}
	st := l.FaucetStats()
	if st.GrantsPending != 1 || st.GrantsIssued != 2 {
		t.Fatalf("stats after denial: %+v (pending must be 1 — three retries are one denied identity)", st)
	}
	*now += 500 // one token accrues (continuous)
	if !l.CanPublish(late) || l.Balance(late) != 500_000 {
		t.Fatalf("the denied identity was not granted after accrual: can=%v balance=%d", l.CanPublish(late), l.Balance(late))
	}
	if st := l.FaucetStats(); st.GrantsPending != 0 || st.GrantsIssued != 3 {
		t.Fatalf("stats after the retry: %+v", st)
	}
}

// TestR212OwnerIsGrantedUnmetered: the node's own identity receives its grant without
// touching the bucket, once, whether the bucket is full or empty.
func TestR212OwnerIsGrantedUnmetered(t *testing.T) {
	l, _ := r212Ledger(t, 1, 1, 1_000, 0)
	l.CanPublish(r212ID(9)) // drains the single token
	self := r212ID(0)
	l.GrantOwner(self)
	if l.Balance(self) != 500_000 {
		t.Fatalf("owner balance = %d after GrantOwner on an EMPTY bucket, want 500,000", l.Balance(self))
	}
	l.GrantOwner(self)
	if l.Balance(self) != 500_000 {
		t.Fatalf("a second GrantOwner granted again: %d", l.Balance(self))
	}
	if st := l.FaucetStats(); st.Level != 0 || st.GrantsIssued != 2 || st.GrantsPending != 0 {
		t.Fatalf("stats: %+v — the owner grant must not consume a token and must count as issued", st)
	}
	// On an UNCONFIGURED ledger GrantOwner is a plain Register.
	u := New(50_000, 500_000)
	u.GrantOwner(self)
	if u.Balance(self) != 500_000 || u.FaucetStats().Configured {
		t.Fatalf("unconfigured GrantOwner: balance %d configured %v", u.Balance(self), u.FaucetStats().Configured)
	}
}

// TestR212NonSpendPathsConsumeNoToken: a fetcher credited fetched bytes, a bond-challenged
// peer, a PoR prover and a bounty-paid repairer are registered PENDING and take nothing
// from the bucket (PE S1: the bond-audit sweep alone would otherwise drain it every 60 s).
func TestR212NonSpendPathsConsumeNoToken(t *testing.T) {
	l, _ := r212Ledger(t, 5, 5, 1_000, 0)
	server, fetcher, prover, bonded := r212ID(1), r212ID(2), r212ID(3), r212ID(4)
	l.GrantOwner(server)
	l.RecordServe(server, fetcher, ports.HashBytes([]byte("c")), 4096)
	l.RecordAudit(prover, ports.HashBytes([]byte("c")), true)
	l.RecordBondChallenge(bonded, ports.HashBytes([]byte("r")), 1<<20, true, 7)
	_ = l.Reputation(bonded)
	st := l.FaucetStats()
	if st.Level != 5 {
		t.Fatalf("a non-spend path consumed a token: level %d, want 5", st.Level)
	}
	if st.GrantsPending != 3 {
		t.Fatalf("pending = %d, want 3 (fetcher, prover, bonded peer registered at zero)", st.GrantsPending)
	}
	if l.Balance(fetcher) != 0 || l.Balance(bonded) != 0 {
		t.Fatalf("a non-spending identity holds a balance: fetcher %d bonded %d", l.Balance(fetcher), l.Balance(bonded))
	}
	if l.Balance(prover) != l.AuditReward {
		t.Fatalf("the prover's audit reward was not credited independent of the grant: %d", l.Balance(prover))
	}
}

// TestR212DenyFloorDegradesInsteadOfDenying: with a floor set, an identity that finds the
// bucket empty receives the floor once, is no longer pending, and is counted as degraded;
// the farm's per-identity yield drops from the grant to the floor while onboarding never
// reaches zero (PE §4's "degrade" option, built beside "deny"; the owner picks).
func TestR212DenyFloorDegradesInsteadOfDenying(t *testing.T) {
	l, _ := r212Ledger(t, 1, 1, 1_000, 50_000)
	l.CanPublish(r212ID(1)) // takes the one token
	d := r212ID(2)
	if !l.CanPublish(d) {
		t.Fatalf("with a floor of one fee, a degraded identity must be able to publish once")
	}
	if l.Balance(d) != 50_000 {
		t.Fatalf("degraded balance = %d, want the 50,000 floor", l.Balance(d))
	}
	st := l.FaucetStats()
	if st.GrantsDegraded != 1 || st.GrantsPending != 0 || st.GrantsIssued != 1 || st.DenyFloor != 50_000 {
		t.Fatalf("stats: %+v", st)
	}
	if err := l.ChargePublish(d); err != nil || l.Balance(d) != 0 {
		t.Fatalf("the degraded identity's one publish: err %v balance %d", err, l.Balance(d))
	}
	if err := l.ChargePublish(d); err != ports.ErrInsufficientCredit {
		t.Fatalf("a degraded identity was granted twice: %v", err)
	}
}

// TestR212UnconfiguredLedgerIsThePreR212Faucet pins the default: with no SetFaucet, Register
// grants at first touch, no account is ever pending, and FaucetStats reports unconfigured.
func TestR212UnconfiguredLedgerIsThePreR212Faucet(t *testing.T) {
	l := New(50_000, 500_000)
	l.Register(r212ID(1))
	if l.accounts[r212ID(1)].balance != 500_000 || l.accounts[r212ID(1)].grantPending {
		t.Fatalf("unconfigured Register: %+v", l.accounts[r212ID(1)])
	}
	if st := l.FaucetStats(); st.Configured || st.GrantsPending != 0 {
		t.Fatalf("unconfigured stats: %+v", st)
	}
	// A rejected configuration leaves it unconfigured too — never a bucket that cannot refill.
	l.SetFaucet(0, 10, 1_000, 0, func() int64 { return 0 })
	if l.faucet != nil {
		t.Fatalf("SetFaucet with capacity 0 built a bucket")
	}
}
