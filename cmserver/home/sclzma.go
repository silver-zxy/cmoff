package home

import (
	"encoding/binary"
	"errors"
)

// The APK's csv_logic/*.csv files are LZMA compressed in Supercell's way:
// the 5 property bytes, then a 4-byte little-endian uncompressed size (the
// usual .lzma header has 8), then the LZMA stream. Go has no LZMA in its
// standard library; this is a plain LZMA decoder after the LZMA SDK's
// reference decoder, enough to read those tables at runtime.

var errLZMA = errors.New("lzma: corrupt data")

// scDecompress decodes a Supercell LZMA file; other data comes back as is
// (CSV files that are not compressed start with text).
func scDecompress(b []byte) ([]byte, error) {
	if len(b) < 9 || b[0] >= 9*5*5 {
		return b, nil
	}
	props := int(b[0])
	dict := binary.LittleEndian.Uint32(b[1:5])
	size := binary.LittleEndian.Uint32(b[5:9])
	if dict == 0 || size > 64<<20 {
		return b, nil
	}
	return lzmaDecode(b[9:], props, int(size))
}

const (
	lzmaStates     = 12
	lzmaPosBitsMax = 4
	lzmaLenLow     = 3
	lzmaLenMid     = 3
	lzmaLenHigh    = 8
	lzmaEndPosIdx  = 14
	lzmaFullDist   = 128
	lzmaAlignBits  = 4
	lzmaProbInit   = 1024
)

type rangeDec struct {
	in    []byte
	pos   int
	rng   uint32
	code  uint32
	error bool
}

func (r *rangeDec) byte() byte {
	if r.pos >= len(r.in) {
		r.error = true
		return 0
	}
	c := r.in[r.pos]
	r.pos++
	return c
}

func (r *rangeDec) init() {
	r.rng = 0xFFFFFFFF
	for i := 0; i < 5; i++ {
		r.code = r.code<<8 | uint32(r.byte())
	}
}

func (r *rangeDec) normalize() {
	if r.rng < 1<<24 {
		r.rng <<= 8
		r.code = r.code<<8 | uint32(r.byte())
	}
}

func (r *rangeDec) bit(p *uint16) uint32 {
	bound := (r.rng >> 11) * uint32(*p)
	var b uint32
	if r.code < bound {
		r.rng = bound
		*p += (2048 - *p) >> 5
	} else {
		r.rng -= bound
		r.code -= bound
		*p -= *p >> 5
		b = 1
	}
	r.normalize()
	return b
}

func (r *rangeDec) direct(n int) uint32 {
	var res uint32
	for ; n > 0; n-- {
		r.rng >>= 1
		r.code -= r.rng
		t := 0 - (r.code >> 31)
		r.code += r.rng & t
		res = res<<1 + t + 1
		r.normalize()
	}
	return res
}

func bitTree(r *rangeDec, probs []uint16, bits int) uint32 {
	m := uint32(1)
	for i := 0; i < bits; i++ {
		m = m<<1 + r.bit(&probs[m])
	}
	return m - 1<<bits
}

func bitTreeReverse(r *rangeDec, probs []uint16, bits int) uint32 {
	m, sym := uint32(1), uint32(0)
	for i := 0; i < bits; i++ {
		b := r.bit(&probs[m])
		m = m<<1 + b
		sym |= b << i
	}
	return sym
}

type lenDec struct {
	choice, choice2 uint16
	low             [1 << lzmaPosBitsMax][1 << lzmaLenLow]uint16
	mid             [1 << lzmaPosBitsMax][1 << lzmaLenMid]uint16
	high            [1 << lzmaLenHigh]uint16
}

func newLenDec() *lenDec {
	l := &lenDec{choice: lzmaProbInit, choice2: lzmaProbInit}
	for i := range l.low {
		for j := range l.low[i] {
			l.low[i][j] = lzmaProbInit
			l.mid[i][j] = lzmaProbInit
		}
	}
	for i := range l.high {
		l.high[i] = lzmaProbInit
	}
	return l
}

func (l *lenDec) decode(r *rangeDec, posState uint32) uint32 {
	if r.bit(&l.choice) == 0 {
		return bitTree(r, l.low[posState][:], lzmaLenLow)
	}
	if r.bit(&l.choice2) == 0 {
		return 1<<lzmaLenLow + bitTree(r, l.mid[posState][:], lzmaLenMid)
	}
	return 1<<lzmaLenLow + 1<<lzmaLenMid + bitTree(r, l.high[:], lzmaLenHigh)
}

func probs(n int) []uint16 {
	p := make([]uint16, n)
	for i := range p {
		p[i] = lzmaProbInit
	}
	return p
}

func lzmaDecode(in []byte, props, size int) ([]byte, error) {
	lc, lp, pb := props%9, props/9%5, props/45
	out := make([]byte, 0, size)
	r := &rangeDec{in: in}
	r.init()

	literal := probs(0x300 << (lc + lp))
	isMatch := probs(lzmaStates << lzmaPosBitsMax)
	isRep := probs(lzmaStates)
	isRepG0 := probs(lzmaStates)
	isRepG1 := probs(lzmaStates)
	isRepG2 := probs(lzmaStates)
	isRep0Long := probs(lzmaStates << lzmaPosBitsMax)
	posSlot := make([][]uint16, 4)
	for i := range posSlot {
		posSlot[i] = probs(1 << 6)
	}
	posDec := probs(1 + lzmaFullDist - lzmaEndPosIdx)
	align := probs(1 << lzmaAlignBits)
	lenD, repLenD := newLenDec(), newLenDec()

	var state, rep0, rep1, rep2, rep3 uint32
	pbMask := uint32(1)<<pb - 1
	lpMask := uint32(1)<<lp - 1
	for len(out) < size {
		if r.error {
			return nil, errLZMA
		}
		pos := uint32(len(out))
		posState := pos & pbMask
		if r.bit(&isMatch[state<<lzmaPosBitsMax+posState]) == 0 {
			var prev uint32
			if len(out) > 0 {
				prev = uint32(out[len(out)-1])
			}
			base := 0x300 * ((pos&lpMask)<<lc + prev>>(8-lc))
			p := literal[base : base+0x300]
			sym := uint32(1)
			if state >= 7 {
				if int(rep0) >= len(out) {
					return nil, errLZMA
				}
				match := uint32(out[len(out)-int(rep0)-1])
				for sym < 0x100 {
					mb := (match >> 7) & 1
					match <<= 1
					b := r.bit(&p[(1+mb)<<8+sym])
					sym = sym<<1 | b
					if mb != b {
						break
					}
				}
			}
			for sym < 0x100 {
				sym = sym<<1 | r.bit(&p[sym])
			}
			out = append(out, byte(sym))
			switch {
			case state < 4:
				state = 0
			case state < 10:
				state -= 3
			default:
				state -= 6
			}
			continue
		}
		var length uint32
		if r.bit(&isRep[state]) != 0 {
			if len(out) == 0 {
				return nil, errLZMA
			}
			if r.bit(&isRepG0[state]) == 0 {
				if r.bit(&isRep0Long[state<<lzmaPosBitsMax+posState]) == 0 {
					if state < 7 {
						state = 9
					} else {
						state = 11
					}
					if int(rep0) >= len(out) {
						return nil, errLZMA
					}
					out = append(out, out[len(out)-int(rep0)-1])
					continue
				}
			} else {
				var dist uint32
				if r.bit(&isRepG1[state]) == 0 {
					dist = rep1
				} else {
					if r.bit(&isRepG2[state]) == 0 {
						dist = rep2
					} else {
						dist = rep3
						rep3 = rep2
					}
					rep2 = rep1
				}
				rep1 = rep0
				rep0 = dist
			}
			length = repLenD.decode(r, posState)
			if state < 7 {
				state = 8
			} else {
				state = 11
			}
		} else {
			rep3, rep2, rep1 = rep2, rep1, rep0
			length = lenD.decode(r, posState)
			if state < 7 {
				state = 7
			} else {
				state = 10
			}
			lenState := min(length, 3)
			slot := bitTree(r, posSlot[lenState], 6)
			if slot < 4 {
				rep0 = slot
			} else {
				bits := int(slot>>1) - 1
				dist := (2 | slot&1) << bits
				if slot < lzmaEndPosIdx {
					dist += bitTreeReverse(r, posDec[dist-slot:], bits)
				} else {
					dist += r.direct(bits-lzmaAlignBits) << lzmaAlignBits
					dist += bitTreeReverse(r, align, lzmaAlignBits)
				}
				rep0 = dist
				if rep0 == 0xFFFFFFFF {
					break // end marker
				}
			}
		}
		length += 2
		if int(rep0) >= len(out) {
			return nil, errLZMA
		}
		for i := uint32(0); i < length && len(out) < size; i++ {
			out = append(out, out[len(out)-int(rep0)-1])
		}
	}
	return out, nil
}
