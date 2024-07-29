package ncrlite

import (
	"bytes"
	"math/rand"
	"slices"
	"testing"
)

func BenchmarkDecompress(b *testing.B) {
	b.StopTimer()

	N := 735000000
	k := 13000000

	buf := new(bytes.Buffer)
	ret := sample(N, k)
	Compress(buf, ret)
	xs := buf.Bytes()

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		buf.Write(xs)
		Decompress(buf)
	}
}

func BenchmarkCompress(b *testing.B) {
	b.StopTimer()

	N := 735000000
	k := 13000000

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

func TestEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	ret := []uint64{}
	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}

func TestMaxUint64(t *testing.T) {
	buf := new(bytes.Buffer)
	ret := []uint64{0xffffffffffffffff}
	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}

func TestLargeUnbalancedCode(t *testing.T) {
	buf := new(bytes.Buffer)
	ret := []uint64{0xfffffffffffffffe, 0xfffffffffffffffd}
	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}

func TestLargeBalancedCode(t *testing.T) {
	buf := new(bytes.Buffer)
	ret := []uint64{}
	for i := 0; i < 64; i++ {
		ret = append(ret, uint64(1)<<i)
	}
	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}

func TestWebPKI(t *testing.T) {
	N := 735000000
	k := 13000000
	// k := 16777217
	// k := 3

	buf := new(bytes.Buffer)

	ret := sample(N, k)

	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}

func TestJustOneBitlength(t *testing.T) {
	buf := new(bytes.Buffer)

	ret := []uint64{0, 1, 2, 3, 4, 5}

	Compress(buf, ret)
	ret2, err := Decompress(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ret, ret2) {
		t.Fatalf("%v %v", ret, ret2)
	}
}
