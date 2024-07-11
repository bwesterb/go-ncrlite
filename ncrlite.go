package ncrlite

import (
	"io"
	"math"
	"math/bits"
	"slices"
)

// Writes a compressed version of set to w.
//
// Assumes no duplicates in set. Forgets about the order.
func Compress(w io.Writer, set []uint64) error {
	slices.Sort(set)
	return CompressSorted(w, set)
}

// Writes a compressed version of set to w.
//
// Assumes set is sorted and has no duplictes.
func CompressSorted(w io.Writer, set []uint64) error {
	bw := newBitWriter(w)

	bw.WriteEliasDelta(uint64(len(set)))

	if err := bw.Err(); err != nil {
		return err
	}

	if len(set) == 0 {
		return nil
	}

	// Compute deltas
	ds := make([]uint64, len(set))

	ds[0] = set[0]
	for i := 0; i < len(ds)-1; i++ {
		if set[i+1] <= set[i] {
			panic("set has duplicates or is not sorted")
		}

		ds[i+1] = set[i+1] - set[i]
	}

	// Compute bitlength counts of deltas
	freq := []int{}
	for i := 0; i < len(ds); i++ {
		bn := bits.Len64(ds[i]) - 1
		for bn >= len(freq) {
			freq = append(freq, 0)
		}
		freq[bn]++
	}

	// Compute Huffman code for the bitlengths
	code := buildHuffmanCode(freq)

	// Pack Huffman code
	code.Pack(bw)
	if err := bw.Err(); err != nil {
		return err
	}

	// Pack each delta
	for _, d := range ds {
		bn := bits.Len64(d) - 1

		bw.WriteBits(uint64(code[bn].code), code[bn].length)
		bw.WriteBits(d^(1<<bn), bn)
	}

	return bw.Close()
}

// Decompresses a set of uint64s from r.
//
// The returned slice will be sorted.
func Decompress(r io.Reader) ([]uint64, error) {
	d, err := NewDecompressor(r)
	if err != nil {
		return err
	}
	ret := make([]uint64, d.Remaining())
	err = d.Read(ret)
	if err != nil {
		return err
	}
	return ret, nil
}

type Decompressor struct {
	br        bitReader
	remaining uint64
}

// Returns the number of uint64 remaining to be decompressed.
func (d *Decompressor) Remaining() uint64 {
	return d.remaining
}

// Fill set with decompressed uint64s.
func (d *Decompressor) Read(set []uint64) error {
	d.br = newBitReader(r)
	d.remaining = d.br.ReadEliasDelta()

}

// Decompressor a set of uint64s from r incrementally.
func NewDecompressor(r io.Reader) (*Decompressor, error) {
}
