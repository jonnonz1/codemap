package hash

import "testing"

func TestBlake3Hex(t *testing.T) {
	h1 := Blake3Hex([]byte("hello"))
	h2 := Blake3Hex([]byte("hello"))
	h3 := Blake3Hex([]byte("world"))

	if h1 != h2 {
		t.Error("same input produced different hashes")
	}
	if h1 == h3 {
		t.Error("different inputs produced same hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h1))
	}
}
