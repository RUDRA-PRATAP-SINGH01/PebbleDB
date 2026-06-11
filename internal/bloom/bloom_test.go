package bloom

import "testing"

func TestBloomAddAndContains(t *testing.T) {
	f := New(100, 0.01)
	f.Add([]byte("hello"))
	f.Add([]byte("world"))

	if !f.MayContain([]byte("hello")) {
		t.Error("expected hello to be in bloom filter")
	}
	if !f.MayContain([]byte("world")) {
		t.Error("expected world to be in bloom filter")
	}
	if f.MayContain([]byte("missing")) {
		t.Error("missing key should not be in bloom filter (with high probability)")
	}
}

func TestBloomEncodeDecode(t *testing.T) {
	f := New(50, 0.01)
	f.Add([]byte("a"))
	f.Add([]byte("b"))
	f.Add([]byte("c"))

	data := f.Encode()
	restored := Decode(data)
	if restored == nil {
		t.Fatal("Decode returned nil")
	}
	if !restored.MayContain([]byte("a")) {
		t.Error("restored filter missing key a")
	}
	if !restored.MayContain([]byte("c")) {
		t.Error("restored filter missing key c")
	}
}

func TestBloomDecodeTooShort(t *testing.T) {
	if Decode([]byte{1, 2, 3}) != nil {
		t.Error("expected nil for short data")
	}
}

func TestBloomDecodeRejectsZeroSize(t *testing.T) {
	buf := []byte{0, 0, 0, 1, 0, 0, 0, 0, 0xFF}
	if Decode(buf) != nil {
		t.Fatal("expected nil for m=0 bloom footer")
	}
}

func TestBloomMayContainZeroSizeDoesNotPanic(t *testing.T) {
	f := &Filter{k: 1, m: 0, bits: []byte{}}
	f.MayContain([]byte("key"))
}
