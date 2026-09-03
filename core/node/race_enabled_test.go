//go:build race

package node

// raceEnabled mirrors the runtime's race-instrumentation state, for tests that pair a
// COUNT gate (the property) with a WALL-CLOCK gate (a secondary statement of it). The
// count runs under both; the wall-clock half is skipped under -race, where the detector's
// instrumentation inflates the measurement (108 µs against a 100 µs budget on the CI
// runner, PR #711) and the number would be measuring the detector, not the path.
const raceEnabled = true
