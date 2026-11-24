package pipeline

import "math/bits"

// PaddedLength is the Padmé length an object of n plaintext bytes is padded to before it
// is framed, encrypted and stored.
//
// Padmé (Nikitin, Barman, Lueks, Underwood, Hubaux, Ford, "Reducing Metadata Leakage from
// Encrypted Files and Communication with PURBs", PoPETs 2019(4)) rounds a length up so
// that only its top O(log log n) bits survive: with E = ⌊log2 n⌋ and S = ⌊log2 E⌋ + 1,
// the low E − S bits are cleared upwards. The padded length therefore reveals at most
// O(log log n) bits of n, and the overhead is bounded by 1/2^S, under 12 % for every n
// and under 3.2 % once n reaches 2^16.
//
// silt applies it so that nothing an observer can see about a stored object — the length
// of its shards, how many there are, and the size the registry entry publishes — narrows
// the plaintext length below its Padmé class. The true length travels only inside the
// sealed manifest, which a reader without the link cannot open.
func PaddedLength(n int64) int64 {
	if n <= 1 {
		return max(n, 0)
	}
	e := 63 - bits.LeadingZeros64(uint64(n)) // ⌊log2 n⌋, at least 1 here
	s := 64 - bits.LeadingZeros64(uint64(e)) // ⌊log2 e⌋ + 1
	low := e - s
	if low <= 0 {
		return n
	}
	mask := int64(1)<<low - 1
	return (n + mask) &^ mask
}
