package ncrlite

import (
	"bytes"
	"testing"

	"math/rand"
)

func BenchmarkWebPKI(b *testing.B) {
	b.StopTimer()

	N := 735000000
	// k :=  13000000
	k := 16777217

	buf := new(bytes.Buffer)

	ret := sample(N, k)

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		Compress(buf, ret)
		buf.Reset()
	}
}

func sample(N, k int) []uint64 {
	lut := make(map[uint64]struct{})
	for len(lut) < k {
		x := uint64(rand.Intn(N))
		lut[x] = struct{}{}
	}

	i := 0
	ret := make([]uint64, k)
	for x := range lut {
		ret[i] = x
		i++
	}

	return ret
}

func TestWebPKI(t *testing.T) {
	N := 735000000
	// k :=  13000000
	k := 16777217

	buf := new(bytes.Buffer)

	ret := sample(N, k)

	Compress(buf, ret)
	// t.Fatalf("%d", len(buf.Bytes()))
}
