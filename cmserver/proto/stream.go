package proto

import "encoding/binary"

// Writer builds a Supercell "Titan" message body. The primitive encoders match
// libg.so: writeInt 0x79547c (big-endian int32), writeVInt 0x4d29b0 (signed
// 7-bit varint, sign in bit 6 of the first byte), writeString 0x5ab65c
// (int32 length then bytes, -1 for null), writeBoolean 0x3257a8 (bit-packed,
// 8 to a byte, reset by any non-boolean write), writeBytes 0x686d84.
// LogicLong is two int32s.
type Writer struct {
	b      []byte
	bitPos int // 0 means the next boolean starts a fresh byte
	bitIdx int // index of the byte currently collecting booleans
}

func NewWriter() *Writer { return &Writer{} }

func (w *Writer) Bytes() []byte { return w.b }

func (w *Writer) endBits() { w.bitPos = 0 }

func (w *Writer) Byte(v byte) {
	w.endBits()
	w.b = append(w.b, v)
}

func (w *Writer) Int(v int32) {
	w.endBits()
	var t [4]byte
	binary.BigEndian.PutUint32(t[:], uint32(v))
	w.b = append(w.b, t[:]...)
}

func (w *Writer) Long(hi, lo int32) {
	w.Int(hi)
	w.Int(lo)
}

// LogicLong on the wire inside bodies is two VInts (0x90/0x98 in the stream
// vtable write a LogicLong as two ints; the data-reference encoders use VInt
// pairs). Use VLong where the client writes VInts.
func (w *Writer) VLong(hi, lo int32) {
	w.VInt(hi)
	w.VInt(lo)
}

// Long90 is the stream's 0x90 LogicLong encoder: two big-endian int32s, same
// wire shape as Long (0x98). Kept distinct for a faithful mirror of the
// client's encoders, which call both.
func (w *Writer) Long90(a, b int32) {
	w.Int(a)
	w.Int(b)
}

// DataRef mirrors the client's (class, instance) data-reference encoder
// (0x8e9860): a null reference (both zero) is a single VInt(0); otherwise two
// VInts. The null case is the one the fixed home/avatar sections use.
func (w *Writer) DataRef(class, instance int32) {
	if class == 0 && instance == 0 {
		w.VInt(0)
		return
	}
	w.VInt(class)
	w.VInt(instance)
}

// VInt writes the Titan signed varint.
func (w *Writer) VInt(value int32) {
	w.endBits()
	if value >= 0 {
		switch {
		case value < 0x40:
			w.b = append(w.b, byte(value))
		case value < 0x2000:
			w.b = append(w.b,
				byte((value&0x3F)|0x80),
				byte((value>>6)&0x7F))
		case value < 0x100000:
			w.b = append(w.b,
				byte((value&0x3F)|0x80),
				byte(((value>>6)&0x7F)|0x80),
				byte((value>>13)&0x7F))
		case value < 0x8000000:
			w.b = append(w.b,
				byte((value&0x3F)|0x80),
				byte(((value>>6)&0x7F)|0x80),
				byte(((value>>13)&0x7F)|0x80),
				byte((value>>20)&0x7F))
		default:
			w.b = append(w.b,
				byte((value&0x3F)|0x80),
				byte(((value>>6)&0x7F)|0x80),
				byte(((value>>13)&0x7F)|0x80),
				byte(((value>>20)&0x7F)|0x80),
				byte((value>>27)&0x0F))
		}
		return
	}
	switch {
	case value >= -0x40:
		w.b = append(w.b, byte((value&0x3F)|0x40))
	case value >= -0x2000:
		w.b = append(w.b,
			byte((value&0x3F)|0xC0),
			byte((value>>6)&0x7F))
	case value >= -0x100000:
		w.b = append(w.b,
			byte((value&0x3F)|0xC0),
			byte(((value>>6)&0x7F)|0x80),
			byte((value>>13)&0x7F))
	case value >= -0x8000000:
		w.b = append(w.b,
			byte((value&0x3F)|0xC0),
			byte(((value>>6)&0x7F)|0x80),
			byte(((value>>13)&0x7F)|0x80),
			byte((value>>20)&0x7F))
	default:
		w.b = append(w.b,
			byte((value&0x3F)|0xC0),
			byte(((value>>6)&0x7F)|0x80),
			byte(((value>>13)&0x7F)|0x80),
			byte(((value>>20)&0x7F)|0x80),
			byte((value>>27)&0x0F))
	}
}

func (w *Writer) String(s string) {
	w.endBits()
	if s == "" {
		// Empty string is length 0, not null; null is -1.
		w.Int(0)
		return
	}
	w.Int(int32(len(s)))
	w.b = append(w.b, s...)
}

func (w *Writer) StringNull() {
	w.Int(-1)
}

// RawAppend appends bytes verbatim (no length prefix), ending any boolean run.
func (w *Writer) RawAppend(p []byte) {
	w.endBits()
	w.b = append(w.b, p...)
}

func (w *Writer) RawBytes(p []byte) {
	w.endBits()
	if p == nil {
		w.Int(-1)
		return
	}
	w.Int(int32(len(p)))
	w.b = append(w.b, p...)
}

// Bool packs booleans 8 to a byte; a run continues until the next non-boolean
// write (which calls endBits).
func (w *Writer) Bool(v bool) {
	if w.bitPos == 0 {
		w.b = append(w.b, 0)
		w.bitIdx = len(w.b) - 1
	}
	if v {
		w.b[w.bitIdx] |= 1 << uint(w.bitPos)
	}
	w.bitPos = (w.bitPos + 1) & 7
}

// StringList writes a VInt count then each string.
func (w *Writer) StringList(ss []string) {
	w.VInt(int32(len(ss)))
	for _, s := range ss {
		w.String(s)
	}
}

// Reader parses a Supercell body (for Login fields and client commands).
type Reader struct {
	b       []byte
	i       int
	err     error
	bitPos  int  // 0 means the next boolean reads a fresh byte
	bitByte byte // the byte currently being unpacked into booleans
}

func NewReader(b []byte) *Reader { return &Reader{b: b} }

func (r *Reader) Err() error { return r.err }

// Remaining reports how many bytes are left unread.
func (r *Reader) Remaining() int { return len(r.b) - r.i }

// Skip advances past n bytes (used to jump over fixed encoder sections whose
// length is computed with a throwaway Writer). It ends any boolean run.
func (r *Reader) Skip(n int) {
	r.bitPos = 0
	if r.i+n > len(r.b) {
		r.fail()
		r.i = len(r.b)
		return
	}
	r.i += n
}

func (r *Reader) Int() int32 {
	r.bitPos = 0
	if r.i+4 > len(r.b) {
		r.fail()
		return 0
	}
	v := int32(binary.BigEndian.Uint32(r.b[r.i:]))
	r.i += 4
	return v
}

// Long reads the stream's two-int32 LogicLong (slots 0x90/0x98).
func (r *Reader) Long() (int32, int32) {
	a := r.Int()
	b := r.Int()
	return a, b
}

// VLong reads a LogicLong written as two VInts (0x9052f4: low then high).
func (r *Reader) VLong() (int32, int32) {
	a := r.VInt()
	b := r.VInt()
	return a, b
}

// DataRef reads the (class, instance) reference written by Writer.DataRef: a
// leading VInt of 0 is the null reference (0, 0); otherwise a second VInt
// follows.
func (r *Reader) DataRef() (int32, int32) {
	class := r.VInt()
	if class == 0 {
		return 0, 0
	}
	return class, r.VInt()
}

// Bool reads a bit-packed boolean, mirroring Writer.Bool: the first boolean of
// a run consumes a byte, later ones read higher bits of it, and any non-boolean
// read starts a fresh byte.
func (r *Reader) Bool() bool {
	if r.bitPos == 0 {
		if r.i >= len(r.b) {
			r.fail()
			return false
		}
		r.bitByte = r.b[r.i]
		r.i++
	}
	v := r.bitByte&(1<<uint(r.bitPos)) != 0
	r.bitPos = (r.bitPos + 1) & 7
	return v
}

func (r *Reader) VInt() int32 {
	r.bitPos = 0
	var result int32
	var shift uint
	first := true
	var negative bool
	for {
		if r.i >= len(r.b) {
			r.fail()
			return 0
		}
		c := r.b[r.i]
		r.i++
		if first {
			negative = c&0x40 != 0
			result = int32(c & 0x3F)
			shift = 6
			if c&0x80 == 0 {
				if negative {
					result |= ^int32(0) << 6
				}
				return result
			}
			first = false
			continue
		}
		result |= int32(c&0x7F) << shift
		shift += 7
		if c&0x80 == 0 {
			if negative && shift < 32 {
				result |= ^int32(0) << shift
			}
			return result
		}
	}
}

func (r *Reader) String() string {
	n := r.Int()
	if n < 0 {
		return ""
	}
	if r.i+int(n) > len(r.b) {
		r.fail()
		return ""
	}
	s := string(r.b[r.i : r.i+int(n)])
	r.i += int(n)
	return s
}

func (r *Reader) fail() {
	if r.err == nil {
		r.err = errBounds
	}
}

var errBounds = &streamError{"read past end of body"}

type streamError struct{ s string }

func (e *streamError) Error() string { return e.s }
