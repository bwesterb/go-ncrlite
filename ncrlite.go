package ncrlite

import (
	"errors"
	"fmt"
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
		return bw.Close()
	}

	if len(set) == 1 {
		bw.WriteUvarint(set[0])
		return bw.Close()
	}

	// Compute deltas
	ds := make([]uint64, len(set))

	// None of the other deltas can be zero, so add one. As set contains
	// at least two element, set[0] can't be 2⁶⁴-1, so there is no overflow.
	ds[0] = set[0] + 1
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

		bw.WriteBits(uint64(code[bn].code), int(code[bn].length))

		bw.WriteBits(d^(1<<bn), bn)
	}

	// End with single byte so that when reading we can
	// peek efficiently without hitting EOF.
	bw.WriteBits(0xaa, 8)

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
	size      uint64
	remaining uint64
	l         io.Writer

	tree    htLut  // Huffman tree
	prev    uint64 // last value emitted
	started bool   // true if a value has been emitted
}

// Returns the number of uint64 remaining to be decompressed.
func (d *Decompressor) Remaining() uint64 {
	return d.remaining
}

var ErrNoMore = errors.New("Reading beyond end of set")

// Do the actual reading after having accounted for all error conditions
// and corner cases.
func (d *Decompressor) read(set []uint64) {
	for i := 0; i < len(set); i++ {
		// Read codeword for length
		node := 0
		var entry htLutEntry

		for {
			code := d.br.PeekByte()
			entry = d.tree[node+int(code)]

			if entry.skip != 0 {
				break
			}

			d.br.SkipBits(8)
			node = entry.next
		}

		d.br.SkipBits(entry.skip)

		delta := d.br.ReadBits(entry.value) | (1 << entry.value)

		val := d.prev + delta

		if !d.started {
			val-- // we shifted the first value so it can't be zero as delta
			d.started = true
		}

		d.prev = val
		set[i] = val
	}
}

// Fill set with decompressed uint64s.
func (d *Decompressor) Read(set []uint64) error {
	if len(set) == 0 {
		return nil
	}

	if d.size == 0 {
		return ErrNoMore
	}

	if d.size == 1 {
		if d.remaining == 0 {
			return ErrNoMore
		}

		set[0] = d.br.ReadUvarint()
		if err := d.br.Err(); err != nil {
			return err
		}

		d.remaining = 0

		if len(set) > 1 {
			return ErrNoMore
		}

		return nil
	}

	if d.remaining < uint64(len(set)) {
		return ErrNoMore
	}

	if d.tree == nil {
		for i := 0; i < len(set); i++ {
			val := d.prev + 1

			if !d.started {
				val-- // we shifted the first value so it can't be zero as delta
				d.started = true
			}

			d.prev = val
			set[i] = val
		}
	} else {
		d.read(set)
	}

	d.remaining -= uint64(len(set))

	if d.remaining == 0 {
		if d.br.ReadBits(8) != 0xaa {
			return errors.New("Incorrect endmarker")
		}
	}

	return d.br.Err()
}

// Returns a new Decompressor that reads a set of uint64s from r incrementally.
func NewDecompressor(r io.Reader) (*Decompressor, error) {
	return NewDecompressorWithLogging(r, nil)
}

// Returns a new Decompressor that reads a set of uint64s from r incrementally.
//
// Logs information about the compressed format to l.
func NewDecompressorWithLogging(r io.Reader, l io.Writer) (*Decompressor, error) {
	br := newBitReader(r)
	d := &Decompressor{br: br}

	// Read size of set
	d.size = br.ReadUvarint()
	if err := br.Err(); err != nil {
		return nil, err
	}

	if l != nil {
		fmt.Fprintf(l, "size                 %d\n", d.size)
	}

	d.remaining = d.size

	if d.size <= 1 {
		return d, nil
	}

	// Read Huffman code
	var err error
	d.tree, err = unpackHuffmanTree(br, l)
	if err != nil {
		return nil, err
	}

	return d, nil
}
