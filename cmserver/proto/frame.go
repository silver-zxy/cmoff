package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Frame is one Supercell message: 2-byte id, 3-byte length, 2-byte version,
// then the payload.
type Frame struct {
	ID      uint16
	Version uint16
	Payload []byte
}

func ReadFrame(r io.Reader) (*Frame, error) {
	var h [7]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	id := binary.BigEndian.Uint16(h[0:2])
	n := uint32(h[2])<<16 | uint32(h[3])<<8 | uint32(h[4])
	ver := binary.BigEndian.Uint16(h[5:7])
	if n > 1<<24 {
		return nil, fmt.Errorf("frame %d too long: %d", id, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &Frame{ID: id, Version: ver, Payload: buf}, nil
}

func WriteFrame(w io.Writer, f *Frame) error {
	n := len(f.Payload)
	h := []byte{
		byte(f.ID >> 8), byte(f.ID),
		byte(n >> 16), byte(n >> 8), byte(n),
		byte(f.Version >> 8), byte(f.Version),
	}
	if _, err := w.Write(h); err != nil {
		return err
	}
	_, err := w.Write(f.Payload)
	return err
}
