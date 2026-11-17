package tcpnet

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// feedPieced writes raw to one end of a pipe and runs readPieced on the other,
// returning the frames it completed and the error it stopped with.
func feedPieced(t *testing.T, raw []byte) ([][]byte, error) {
	t.Helper()
	tr, loop := newTransport(t, 740)
	defer func() { tr.Close(); loop.Stop() }()
	a, b := net.Pipe()
	go func() {
		a.Write(raw)
		a.Close()
	}()
	var frames [][]byte
	err := tr.readPieced(b, ports.NodeID{}, func(frame []byte, release func()) {
		frames = append(frames, frame)
		release()
	})
	return frames, err
}

func unit(hdr uint32, payload []byte) []byte {
	b := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(b, hdr)
	copy(b[4:], payload)
	return b
}

// A SMALL FRAME BETWEEN TWO PIECES OF A LARGE ONE arrives whole and FIRST, and the
// large one reassembles byte-for-byte around it. That is the whole point of the
// format, and the order is what a receiver sees on the wire.
func TestPiecedFramesReassembleAroundAnInterleavedSmallFrame(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789abcdef"), (pieceBytes+100)/16+1)
	small := []byte("probe")
	var raw []byte
	raw = append(raw, unit(pieceStart|uint32(len(big)), nil)...)
	raw = append(raw, unit(pieceNext|pieceBytes, big[:pieceBytes])...)
	raw = append(raw, unit(uint32(len(small)), small)...)
	raw = append(raw, unit(pieceNext|uint32(len(big)-pieceBytes), big[pieceBytes:])...)

	frames, _ := feedPieced(t, raw)
	if len(frames) != 2 || !bytes.Equal(frames[0], small) || !bytes.Equal(frames[1], big) {
		t.Fatalf("got %d frames (sizes %v), want the %d-byte small frame then the %d-byte large one intact",
			len(frames), sizes(frames), len(small), len(big))
	}
}

func sizes(fs [][]byte) []int {
	out := make([]int, len(fs))
	for i, f := range fs {
		out[i] = len(f)
	}
	return out
}

// EVERY MALFORMED SEQUENCE ENDS THE CONVERSATION with a named error and completes no
// frame. A reader that tolerated any of these would either assemble bytes from two
// frames into one or allocate for a frame it never bounded.
func TestPiecedFramingRefusesMalformedSequences(t *testing.T) {
	start := unit(pieceStart|(pieceBytes+1), nil)
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	cases := []struct {
		name, want string
		raw        []byte
	}{
		{"a piece with no frame started", "no frame in progress", unit(pieceNext|4, []byte("abcd"))},
		{"a frame started inside another", "inside another", cat(start, start)},
		{"a piece that overruns its frame", "piece of",
			cat(start, unit(pieceNext|pieceBytes, make([]byte, pieceBytes)), unit(pieceNext|2, []byte("xx")))},
		{"a pieced frame small enough to send whole", "pieced frame length", unit(pieceStart|8, nil)},
		{"a pieced frame over the frame cap", "pieced frame length", unit(pieceStart|(maxFrame+1)&^pieceFlags, nil)},
		{"a header with both flags", "both piece flags", unit(pieceFlags|4, nil)},
		{"a zero-length whole frame", "frame length 0", unit(0, nil)},
	}
	for _, c := range cases {
		frames, err := feedPieced(t, c.raw)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: stopped with %v, want an error naming %q", c.name, err, c.want)
		}
		if len(frames) != 0 {
			t.Errorf("%s: completed %d frame(s) from a malformed sequence", c.name, len(frames))
		}
	}
}
