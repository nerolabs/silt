// Package chunk splits a byte stream into fixed-size frames and joins
// them back, exactly.
//
// Every frame is the same size (ChunkSize), which matters later: erasure
// coding needs equal-length shards. The final piece of a file is almost
// never a full chunk, so each frame carries an explicit length header and
// the tail is zero-padded:
//
//	[8-byte big-endian payload length][payload][zero padding]
//
// Join reads the header and takes exactly that many bytes, so
// Join(Split(x)) == x for any x — including empty — without needing any
// out-of-band size information.
package chunk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// HeaderSize is the per-frame framing overhead in bytes.
const HeaderSize = 8

// MinChunkSize is the smallest usable chunk size (header + 1 payload byte).
const MinChunkSize = HeaderSize + 1

var ErrBadFrame = errors.New("chunk: malformed frame")

// Split reads r to EOF and returns frames of exactly chunkSize bytes.
// An empty input yields zero frames.
func Split(r io.Reader, chunkSize int) ([][]byte, error) {
	var frames [][]byte
	if err := SplitEach(r, chunkSize, func(f []byte) error {
		frames = append(frames, f)
		return nil
	}); err != nil {
		return nil, err
	}
	return frames, nil
}

// SplitEach is Split one frame at a time: it hands each frame to fn as soon as it is read,
// so a caller can process a file of any size holding one frame. It is the only framing
// implementation — Split collects its frames — so the two can never disagree on bytes. A
// frame is fn's to keep; SplitEach never reuses it.
func SplitEach(r io.Reader, chunkSize int, fn func(frame []byte) error) error {
	if chunkSize < MinChunkSize {
		return fmt.Errorf("chunk: chunkSize %d < minimum %d", chunkSize, MinChunkSize)
	}
	payloadSize := chunkSize - HeaderSize
	for {
		frame := make([]byte, chunkSize)
		n, err := io.ReadFull(r, frame[HeaderSize:])
		if n == 0 {
			if err == io.EOF || err == io.ErrUnexpectedEOF || err == nil {
				return nil
			}
			return err
		}
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		binary.BigEndian.PutUint64(frame[:HeaderSize], uint64(n))
		if ferr := fn(frame); ferr != nil {
			return ferr
		}
		if n < payloadSize {
			return nil
		}
	}
}

// Join writes the original stream reassembled from frames to w.
//
// The frames must read as a run of full frames, then at most one partial frame, then
// any number of empty frames (header length zero). The empty tail is padding: a
// publisher adds it so the frame COUNT shows only the object's size class, and Join
// writes nothing for it. A full or partial frame after a short one is an error rather
// than a silent truncation, as is any other inconsistency.
func Join(w io.Writer, frames [][]byte) error {
	ended := false // a frame shorter than its room has been seen: only empty frames may follow
	for i, frame := range frames {
		if len(frame) < MinChunkSize {
			return fmt.Errorf("%w: frame %d is %d bytes", ErrBadFrame, i, len(frame))
		}
		n := binary.BigEndian.Uint64(frame[:HeaderSize])
		payloadSize := uint64(len(frame) - HeaderSize)
		if n > payloadSize {
			return fmt.Errorf("%w: frame %d claims %d payload bytes, has room for %d", ErrBadFrame, i, n, payloadSize)
		}
		if ended && n != 0 {
			return fmt.Errorf("%w: frame %d carries %d bytes after the content ended", ErrBadFrame, i, n)
		}
		if n < payloadSize {
			ended = true
		}
		if _, err := w.Write(frame[HeaderSize : HeaderSize+n]); err != nil {
			return err
		}
	}
	return nil
}
