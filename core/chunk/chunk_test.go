package chunk

import (
	"bytes"
	"math/rand"
	"testing"
	"testing/quick"
)

const testChunkSize = 1024 // small so tests cross chunk boundaries often

func roundtrip(t *testing.T, data []byte, chunkSize int) {
	t.Helper()
	frames, err := Split(bytes.NewReader(data), chunkSize)
	if err != nil {
		t.Fatalf("Split(%d bytes): %v", len(data), err)
	}
	for i, f := range frames {
		if len(f) != chunkSize {
			t.Fatalf("frame %d has size %d, want %d", i, len(f), chunkSize)
		}
	}
	var out bytes.Buffer
	if err := Join(&out, frames); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("roundtrip mismatch: %d bytes in, %d bytes out", len(data), out.Len())
	}
}

func TestRoundtripBoundaries(t *testing.T) {
	payload := testChunkSize - HeaderSize
	sizes := []int{
		0, 1, 2,
		payload - 1, payload, payload + 1, // one-chunk boundary
		2*payload - 1, 2 * payload, 2*payload + 1,
		10*payload + 37,
	}
	for _, n := range sizes {
		data := make([]byte, n)
		rand.New(rand.NewSource(int64(n))).Read(data)
		roundtrip(t, data, testChunkSize)
	}
}

func TestRoundtripProperty(t *testing.T) {
	f := func(data []byte, sizeSeed uint16) bool {
		chunkSize := MinChunkSize + int(sizeSeed)%2048
		frames, err := Split(bytes.NewReader(data), chunkSize)
		if err != nil {
			return false
		}
		var out bytes.Buffer
		if err := Join(&out, frames); err != nil {
			return false
		}
		return bytes.Equal(out.Bytes(), data)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyInputYieldsZeroFrames(t *testing.T) {
	frames, err := Split(bytes.NewReader(nil), testChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 0 {
		t.Fatalf("empty input produced %d frames, want 0", len(frames))
	}
}

func TestChunkSizeTooSmall(t *testing.T) {
	if _, err := Split(bytes.NewReader([]byte("x")), HeaderSize); err == nil {
		t.Fatal("expected error for chunkSize <= HeaderSize")
	}
}

func TestJoinRejectsLyingHeader(t *testing.T) {
	frames, _ := Split(bytes.NewReader(make([]byte, 100)), testChunkSize)
	frames[0][0] = 0xFF // claim absurd payload length
	if err := Join(&bytes.Buffer{}, frames); err == nil {
		t.Fatal("expected error for header claiming more payload than frame holds")
	}
}

func TestJoinRejectsShortMiddleFrame(t *testing.T) {
	data := make([]byte, 3*(testChunkSize-HeaderSize))
	frames, _ := Split(bytes.NewReader(data), testChunkSize)
	// Shrink the middle frame's claimed length: reordering/truncation must not pass.
	frames[1][7] = 1
	for i := 0; i < 7; i++ {
		frames[1][i] = 0
	}
	if err := Join(&bytes.Buffer{}, frames); err == nil {
		t.Fatal("expected error for short non-final frame")
	}
}

// TestJoinWritesNothingForTrailingEmptyFrames: frames after the content ends that carry a
// zero-length header are padding, and Join must write nothing for them however their
// payload bytes read.
func TestJoinWritesNothingForTrailingEmptyFrames(t *testing.T) {
	data := make([]byte, testChunkSize+5)
	rand.New(rand.NewSource(2)).Read(data)
	frames, _ := Split(bytes.NewReader(data), testChunkSize)
	for i := 0; i < 3; i++ {
		filler := make([]byte, testChunkSize)
		rand.New(rand.NewSource(int64(10 + i))).Read(filler[HeaderSize:])
		frames = append(frames, filler)
	}
	var out bytes.Buffer
	if err := Join(&out, frames); err != nil {
		t.Fatalf("join with trailing empty frames: %v", err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("join wrote %d bytes, want the %d content bytes", out.Len(), len(data))
	}
}

// TestJoinRejectsContentAfterTheEnd: once a frame is shorter than its room, every later
// frame must be empty. A full or partial frame after it is reordering or splicing.
func TestJoinRejectsContentAfterTheEnd(t *testing.T) {
	data := make([]byte, 2*(testChunkSize-HeaderSize))
	full, _ := Split(bytes.NewReader(data), testChunkSize)
	short, _ := Split(bytes.NewReader([]byte{1, 2, 3}), testChunkSize)
	empty := make([]byte, testChunkSize)
	for _, c := range []struct {
		name   string
		frames [][]byte
	}{
		{"full after short", [][]byte{short[0], full[0]}},
		{"short after short", [][]byte{short[0], short[0]}},
		{"full after empty", [][]byte{full[0], empty, full[1]}},
	} {
		if err := Join(&bytes.Buffer{}, c.frames); err == nil {
			t.Fatalf("%s: Join accepted content after the end", c.name)
		}
	}
}

func BenchmarkSplit64KiB(b *testing.B) {
	data := make([]byte, 8<<20)
	rand.New(rand.NewSource(1)).Read(data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Split(bytes.NewReader(data), 64<<10); err != nil {
			b.Fatal(err)
		}
	}
}
