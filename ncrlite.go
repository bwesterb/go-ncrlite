package ncrlite

import (
	"errors"
	"io"
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

	bw.WriteUvarint(uint64(len(set)))

	if err := bw.Err(); err != nil {
		return err
	}

	if len(set) == 0 {
		return nil
	}

	// Compute deltas
	ds := make([]uint64, len(set))

	ds[0] = set[0] + 1 // none of the other deltas can be zero
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
		return nil, err
	}
	ret := make([]uint64, d.Remaining())
	err = d.Read(ret)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

type Decompressor struct {
	br        *bitReader
	remaining uint64

	root    *htNode // Huffman tree
	prev    uint64  // last value emitted
	started bool    // true if a value has been emitted
}

// Returns the number of uint64 remaining to be decompressed.
func (d *Decompressor) Remaining() uint64 {
	return d.remaining
}

// Fill set with decompressed uint64s.
func (d *Decompressor) Read(set []uint64) error {
	for i := 0; i < len(set); i++ {
		if d.remaining == 0 {
			return errors.New("Reading beyond end of set")
		}

		// Read codeword for length
		j := 0
		node := d.root
		for node.children[0] != nil {
			j++
			node = node.children[d.br.ReadBits(1)]
		}

		delta := d.br.ReadBits(node.value) | (1 << node.value)

		val := d.prev + delta

		if !d.started {
			val-- // we shifted the first value so it can't be zero as delta
			d.started = true
		}

		d.prev = val
		set[i] = val
	}
	return d.br.Err()
}

// Decompressor a set of uint64s from r incrementally.
func NewDecompressor(r io.Reader) (*Decompressor, error) {
	br := newBitReader(r)
	d := &Decompressor{br: br}

	// Read size of set
	d.remaining = br.ReadUvarint()
	if err := br.Err(); err != nil {
		return nil, err
	}

	// Read Huffman code
	var err error
	d.root, err = unpackHuffmanTree(br)
	if err != nil {
		return nil, err
	}

	return d, nil
}
