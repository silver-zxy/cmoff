package proto

import "testing"

func TestVIntRoundTrip(t *testing.T) {
	cases := []int32{0, 1, -1, 0x3F, 0x40, -0x40, -0x41, 0x2000, -0x2000,
		0x100000, -0x100000, 0x7FFFFFFF, 63, 64, 8192, 582, 10101, 25091}
	for _, v := range cases {
		w := NewWriter()
		w.VInt(v)
		r := NewReader(w.Bytes())
		got := r.VInt()
		if r.Err() != nil {
			t.Fatalf("VInt(%d): read err %v", v, r.Err())
		}
		if got != v {
			t.Fatalf("VInt(%d) round-trip got %d (bytes %x)", v, got, w.Bytes())
		}
	}
}

func TestBoolPacking(t *testing.T) {
	w := NewWriter()
	w.Bool(true)
	w.Bool(false)
	w.Bool(true)
	// 3 bits -> one byte: bit0=1, bit2=1 => 0b0000_0101 = 0x05
	if len(w.Bytes()) != 1 || w.Bytes()[0] != 0x05 {
		t.Fatalf("bool packing got %x", w.Bytes())
	}
	w.Int(7) // non-bool resets the bit run
	w.Bool(true)
	if len(w.Bytes()) != 1+4+1 {
		t.Fatalf("expected a fresh byte after Int, got %x", w.Bytes())
	}
}

func TestStringRoundTrip(t *testing.T) {
	w := NewWriter()
	w.String("KnightB1")
	w.Int(5)
	r := NewReader(w.Bytes())
	if r.String() != "KnightB1" || r.Int() != 5 {
		t.Fatal("string round-trip")
	}
}
