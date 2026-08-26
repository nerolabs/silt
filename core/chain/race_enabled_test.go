//go:build race

package chain

// raceEnabled mirrors the runtime's race-instrumentation state for tests whose
// assertions are MEMORY PROFILES: race shadow memory and pool-reuse suppression
// inflate the measured heap (~10× on the #563 bench), so a live-heap budget is
// only meaningful uninstrumented.
const raceEnabled = true
