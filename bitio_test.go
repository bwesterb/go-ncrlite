package ncrlite

import (
	"bytes"
	"testing"
)

func TestEliasDelta(t *testing.T) {
	buf := new(bytes.Buffer)

	w := newBitWriter(buf)
	for i := uint64(0); i < 1000; i++ {
		w.WriteEliasDelta(i)
	}
	err := w.Close()
	if err != nil {
		t.Fatal(err)
	}

	r := newBitReader(buf)
	for i := uint64(0); i < 1000; i++ {
		j := r.ReadEliasDelta()
		if i != j {
			t.Fatalf("%d ≠ %d", i, j)
		}

		if r.Err() != nil {
			t.Fatal()
		}
	}
}
