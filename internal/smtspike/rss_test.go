package smtspike

import (
	"os"
	"strconv"
	"strings"
)

// residentMB reports the process resident set size in MiB — the number that
// actually answers the OOM question the node-store measurement exists for.
//
// Go's runtime.MemStats.HeapAlloc counts only the Go heap. bbolt is mmap'd, so
// the pages it touches live in the OS page cache OUTSIDE the Go heap — invisible
// to HeapAlloc and precisely the residency the PE flagged as the reason to
// prefer bbolt (kernel-evictable) over an LSM's server-sized heap caches.
// Measuring heap alone would compare the two backends on the wrong axis.
//
// On Linux (the floor box) this reads /proc/self/statm, whose second field is
// the resident page count. On other platforms it returns 0, and the caller
// notes the number is unavailable — the laptop run is shape only; the floor box
// is Linux.
func residentMB() float64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0 // not Linux — floor box is, laptop isn't
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return float64(pages*int64(os.Getpagesize())) / (1 << 20)
}
