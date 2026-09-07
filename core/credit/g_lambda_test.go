package credit

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// G-R212-7 numéraire gates inside core/credit (cert §8.1, owner-ratified 2026-09-06).

// TestGLambda4ChunkingInvariance: serving N bytes in M calls mints ⌊N/Dλ⌋ (plain path)
// and the same two floors (object path) independent of M (G-λ-4). Ablation: a per-call
// floor ⇒ RED (M = 64 calls of 64 KiB mint 0, expects 10).
func TestGLambda4ChunkingInvariance(t *testing.T) {
	const N = 10*ServeMintBytesPerCredit + 12_345
	for _, M := range []int64{1, 7, 64} {
		l := New(50_000, 0)
		server, req := id(1), id(2)
		per := N / M
		sent := int64(0)
		for i := int64(0); i < M; i++ {
			b := per
			if i == M-1 {
				b = N - sent
			}
			l.RecordServe(server, req, ports.ChunkID{byte(i)}, b)
			sent += b
		}
		if got := l.Balance(server); got != N/ServeMintBytesPerCredit {
			t.Fatalf("plain path, M=%d: minted %d, want ⌊N/Dλ⌋ = %d — the mint depends on how the bytes were chunked", M, got, N/ServeMintBytesPerCredit)
		}
		if got := l.accounts[server].serveRemainder; got != N%ServeMintBytesPerCredit {
			t.Fatalf("plain path, M=%d: remainder %d, want %d", M, got, N%ServeMintBytesPerCredit)
		}
		// Object path: 3 units → net 21, skim 3, however the bytes arrive.
		lo := New(50_000, 0)
		obj := ports.HashBytes([]byte("g4-obj"))
		const NO = 3 * mintUnit
		sent = 0
		for i := int64(0); i < M; i++ {
			b := NO / M
			if i == M-1 {
				b = NO - sent
			}
			lo.RecordServeToObject(server, req, obj, ports.ChunkID{byte(i)}, b)
			sent += b
		}
		if lo.Balance(server) != 21 || lo.EscrowBalance(obj) != 3 {
			t.Fatalf("object path, M=%d: net %d skim %d, want 21 / 3", M, lo.Balance(server), lo.EscrowBalance(obj))
		}
	}
}

// TestGLambda5SupersedeLeavesNoRemainder: serve a lane to just under a mint boundary,
// redeem a witnessed receipt (the lane dies), serve again — the post-redeem lane starts
// at ZERO. Ablation: keep the remainder on the account ⇒ one extra credit appears (G-λ-5),
// the only double-pay path this mechanism has.
func TestGLambda5SupersedeLeavesNoRemainder(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	obj := ports.HashBytes([]byte("g5-obj"))
	// One Dλ on the lane: the server leg needs 8·Dλ/7 for its first credit — nothing mints.
	l.RecordServeToObject(server, fetcher, obj, ports.ChunkID{1}, ServeMintBytesPerCredit)
	if l.Balance(server) != 0 {
		t.Fatalf("setup: %d minted below the boundary", l.Balance(server))
	}
	paid := l.RedeemDeliveryCredit(server, fetcher, obj, nil, 0)
	if paid <= 0 {
		t.Fatalf("setup: redeem paid %d", paid)
	}
	// Serve one Dλ again on the SAME (server, fetcher, obj): a fresh lane. With the
	// remainder carried on the account the two serves would total 2·Dλ and mint 1.
	l.RecordServeToObject(server, fetcher, obj, ports.ChunkID{2}, ServeMintBytesPerCredit)
	if got := l.Balance(server); got != paid {
		t.Fatalf("balance %d after redeem + re-serve, want exactly the paid leg %d — a remainder survived the supersede and minted for bytes the receipt already paid", got, paid)
	}
	if p, ok := l.provisional[provKey{server: server, requester: fetcher, root: obj}]; !ok || p.bytes != ServeMintBytesPerCredit {
		t.Fatalf("post-redeem lane bytes = %+v, want a fresh lane holding exactly the second serve", p)
	}
}

// TestGLambda6RepairBountyBaseIsPricedInTheFetchPrice: base == c·k·shardBytes/(U/p)
// (G-λ-6): a 64 MiB production chunk at k = 10 pays 256 credits; k·shardBytes one byte
// short of a credit pays 0. Ablation: leave the base at k·shardBytes ⇒ RED (67 million).
func TestGLambda6RepairBountyBaseIsPricedInTheFetchPrice(t *testing.T) {
	if got := RepairBountyBase(10, 6_710_887); got != 256 { // ⌈64 MiB / 10⌉ per shard
		t.Fatalf("64 MiB chunk, k=10: base %d, want 256 credits (k·shardBytes / 262,144)", got)
	}
	if got := RepairBountyBase(10, 26_214); got != 0 {
		t.Fatalf("k·shardBytes = 262,140 < one credit of fetch: base %d, want 0", got)
	}
	if got := RepairBountyBase(10, 26_215); got != 1 {
		t.Fatalf("k·shardBytes = 262,150: base %d, want 1", got)
	}
	if want := int64(10) * 6_710_887 * RepairBountyCoeffNum / RepairBountyCoeffDen / DeliveryBytesPerCredit; RepairBountyBase(10, 6_710_887) != want {
		t.Fatalf("base is not c·k·shardBytes/(U/p)")
	}
	if MinBountyStripeBytes != DeliveryBytesPerCredit {
		t.Fatalf("MinBountyStripeBytes %d != U/p %d", MinBountyStripeBytes, DeliveryBytesPerCredit)
	}
}

// TestRepairBountyBaseAtTheFormerDefaultIsTwoNotZero — the geometry correction (Economist
// advisory 2026-09-06): a shard is a WHOLE ciphertext chunk, so the 64 KiB FORMER default's
// stripe is 10 × 65,552 = 655,520 B and paid a base of 2 — not the zero the G-R212-7 build
// and its blind PE stated on a shard = chunk/k model — with a 20 % truncation (exact 2.5006
// → 2, R-BOUNTY-TRUNCATION), which is one reason the default moved to 256 KiB (a base of
// exactly 10; D-R2.9-NODE-HALF-CALLS 4′). The publish threshold derives from the same
// arithmetic: ~26 KB at k = 10.
func TestRepairBountyBaseAtTheFormerDefaultIsTwoNotZero(t *testing.T) {
	const overhead = 16 // crypto.Overhead, duplicated: core/credit imports no cipher
	if got := RepairBountyBase(10, 65_536+overhead); got != 2 {
		t.Fatalf("64 KiB former default, k=10: base %d, want 2 (655,520 / 262,144 = 2.5006 truncated)", got)
	}
	if got := RepairBountyBase(10, 262_144+overhead); got != 10 {
		t.Fatalf("256 KiB chunk, k=10: base %d, want 10", got)
	}
	min := MinBountyChunkBytesFor(10, overhead)
	if min != 26_199 {
		t.Fatalf("min bounty chunk at k=10: %d, want 26,199 (= ⌈262,144/10⌉ − 16)", min)
	}
	if RepairBountyBase(10, min+overhead) != 1 || RepairBountyBase(10, min-1+overhead) != 0 {
		t.Fatalf("the threshold is not the boundary: base(min) %d, base(min−1) %d", RepairBountyBase(10, min+overhead), RepairBountyBase(10, min-1+overhead))
	}
}

// TestGLambda7SkimAccumulatesAcrossServes: the escrow leg floors the LANE's byte
// accumulator, so eight serves of one Dλ each fund one credit of durability (G-λ-7).
// Ablation: a per-call skim floor ⇒ RED (0).
func TestGLambda7SkimAccumulatesAcrossServes(t *testing.T) {
	l := New(50_000, 0)
	server, req := id(1), id(2)
	obj := ports.HashBytes([]byte("g7-obj"))
	var skimmed int64
	for i := 0; i < SkimDen; i++ {
		skimmed += l.RecordServeToObject(server, req, obj, ports.ChunkID{byte(i)}, ServeMintBytesPerCredit)
	}
	if skimmed != 1 || l.EscrowBalance(obj) != 1 {
		t.Fatalf("eight sub-boundary serves skimmed %d (escrow %d), want 1 — the skim floors per call instead of over the lane", skimmed, l.EscrowBalance(obj))
	}
	if l.Balance(server) != SkimDen-SkimNum {
		t.Fatalf("server net %d, want %d", l.Balance(server), SkimDen-SkimNum)
	}
}

// TestGLambda9ByteObservablesStayBytes: ServedBytes / FetchedBytes are bytes at any Dλ —
// the R2.9a census estimand (G-λ-9). Ablation: route either through the mint ⇒ RED.
func TestGLambda9ByteObservablesStayBytes(t *testing.T) {
	l := New(50_000, 0)
	server, req := id(1), id(2)
	obj := ports.HashBytes([]byte("g9-obj"))
	l.RecordServe(server, req, ports.ChunkID{1}, 12_345)
	l.RecordServeToObject(server, req, obj, ports.ChunkID{2}, 54_321)
	if l.ServedBytes(server) != 12_345+54_321 || l.FetchedBytes(req) != 12_345+54_321 {
		t.Fatalf("served %d fetched %d, want %d bytes each", l.ServedBytes(server), l.FetchedBytes(req), 12_345+54_321)
	}
	if l.Balance(server) != 0 {
		t.Fatalf("66,666 bytes minted %d, want 0 (below Dλ)", l.Balance(server))
	}
}

// TestGLambdaServeMintTelemetry: the instrument the Economist asked for (cert §8.1).
func TestGLambdaServeMintTelemetry(t *testing.T) {
	l := New(50_000, 0)
	server, req := id(1), id(2)
	obj := ports.HashBytes([]byte("tel-obj"))
	l.RecordServe(server, req, ports.ChunkID{1}, 100)                       // zero mint, remainder 100
	l.RecordServe(server, req, ports.ChunkID{2}, 2*ServeMintBytesPerCredit) // mints 2
	l.RecordServeToObject(server, req, obj, ports.ChunkID{3}, mintUnit)     // 7 + 1
	st := l.ServeMintStats()
	if st.BytesPerCredit != ServeMintBytesPerCredit || st.ServedBytes != 100+2*ServeMintBytesPerCredit+mintUnit || st.MintedCredits != 9 || st.SkimmedCredits != 1 || st.ZeroMintServes != 1 {
		t.Fatalf("telemetry %+v", st)
	}
	if st.RemainderBytesServerLeg != 100 || st.RemainderBytesEscrowLeg != 0 {
		t.Fatalf("remainders server %d escrow %d, want 100 / 0 (the lane is exactly on a unit boundary)", st.RemainderBytesServerLeg, st.RemainderBytesEscrowLeg)
	}
	// Both legs report their own waiting bytes (PE M3): one Dλ on a fresh lane mints
	// nothing on either leg, so both legs wait on all of it.
	l.RecordServeToObject(server, req, ports.HashBytes([]byte("tel-obj-2")), ports.ChunkID{4}, ServeMintBytesPerCredit)
	st = l.ServeMintStats()
	if st.RemainderBytesServerLeg != 100+ServeMintBytesPerCredit || st.RemainderBytesEscrowLeg != ServeMintBytesPerCredit {
		t.Fatalf("remainders server %d escrow %d after a sub-boundary lane serve", st.RemainderBytesServerLeg, st.RemainderBytesEscrowLeg)
	}
	// The mint counters are GROSS of reversal; a witnessed redeem reverses the lane and
	// lands in ReversedCredits instead of decrementing them.
	l.RedeemDeliveryCredit(server, req, obj, nil, 0)
	st = l.ServeMintStats()
	if st.MintedCredits != 9 || st.SkimmedCredits != 1 || st.ReversedCredits != 8 {
		t.Fatalf("after a reversing redeem: minted %d skimmed %d reversed %d, want 9 / 1 / 8", st.MintedCredits, st.SkimmedCredits, st.ReversedCredits)
	}
}
