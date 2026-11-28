package main

import "testing"

func TestParseByteRange(t *testing.T) {
	const size = 1000
	ok := []struct {
		spec   string
		off, n int64
	}{
		{"bytes=0-99", 0, 100},
		{"bytes=900-", 900, 100},
		{"bytes=-100", 900, 100},
		{"bytes=-5000", 0, 1000},
		{"bytes=990-2000", 990, 10},
		{"bytes=999-999", 999, 1},
		{" bytes = 10 - 19 ", 10, 10},
	}
	for _, c := range ok {
		off, n, err := parseByteRange(c.spec, size)
		if err != nil || off != c.off || n != c.n {
			t.Errorf("%q: got (%d, %d, %v), want (%d, %d)", c.spec, off, n, err, c.off, c.n)
		}
	}
	for _, spec := range []string{"bytes=1000-", "bytes=5-4", "bytes=0-1,5-6", "items=0-5", "bytes=-", "bytes=-0", "bytes=abc-", "bytes"} {
		if _, _, err := parseByteRange(spec, size); err == nil {
			t.Errorf("%q was accepted", spec)
		}
	}
	if _, _, err := parseByteRange("bytes=0-", 0); err == nil {
		t.Error("a range of an empty file was accepted")
	}
}
