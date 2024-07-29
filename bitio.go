package ncrlite

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

type bitReader struct {
	r    *bufio.Reader
	size byte
	buf  uint64
	err  error

	scratch [8]byte // to prevent allocations
}

type bitWriter struct {
	w      *bufio.Writer
	offset int
	buf    uint64
	err    error
}

var errClosed = errors.New("bitWriter is closed")

func newBitReader(r io.Reader) *bitReader {
	return &bitReader{
		r: bufio.NewReader(r),
	}
}

func newBitWriter(w io.Writer) *bitWriter {
	return &bitWriter{
		w: bufio.NewWriter(w),
	}
}

func (w *bitWriter) Err() error {
	return w.err
}

func (r *bitReader) Err() error {
	return r.err
}

// Returns offset in current byte
func (w *bitWriter) BitOffset() byte {
	return byte(w.offset)
}

func (w *bitWriter) Close() error {
	if w.err != nil {
		return w.err
	}

	for w.offset > 0 {
		w.err = w.w.WriteByte(byte(w.buf))
		w.buf >>= 8
		w.offset -= 8

		if w.err != nil {
			return w.err
		}
	}

	w.err = w.w.Flush()
	if w.err != nil {
		return w.err
	}
	return nil
}

func (w *bitWriter) WriteBits(bs uint64, l int) {
	if w.err != nil {
		return
	}

	w.buf |= (bs << w.offset)

	if w.offset+l < 64 {
		w.offset += l
		return
	}

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], w.buf)
	_, err := w.w.Write(buf[:])
	if err != nil {
		w.err = err
		return
	}

	l2 := 64 - w.offset
	w.buf = bs >> l2
	w.offset = l - l2
}

// Reads bits assuming l <= r.size.
func (r *bitReader) readBits(l byte) uint64 {
	ret := r.buf & (uint64(1<<l) - 1)
	r.size -= l
	r.buf >>= l
	return ret
}

func (r *bitReader) fill() bool {
	n, err := r.r.Read(r.scratch[:])
	if n == 0 {
		r.err = err
		return false
	}

	// An io.Reader is allowed to use whole of buf as scratch space, so we
	// need to explicitly set to zero.
	for i := n; i < 8; i++ {
		r.scratch[i] = 0
	}

	r.buf = binary.LittleEndian.Uint64(r.scratch[:])
	r.size = byte(8 * n)
	return true
}

func (r *bitReader) ReadBit() byte {
	if r.size == 0 {
		if !r.fill() {
			return 0
		}
	}

	ret := byte(r.buf) & 1
	r.size--
	r.buf >>= 1

	return ret
}

// Return the next byte that will be read.
func (r *bitReader) PeekByte() byte {
	for 8 > r.size {
		n, err := r.r.Read(r.scratch[:4])
		if n == 0 {
			r.err = err
			return 0
		}

		// An io.Reader is allowed to use whole of buf as scratch space, so we
		// need to explicitly set to zero.
		for i := n; i < 4; i++ {
			r.scratch[i] = 0
		}

		r.buf |= uint64(binary.LittleEndian.Uint32(r.scratch[:])) << r.size
		r.size += byte(8 * n)
	}

	return byte(r.buf)
}

// Read l bits from r. Assumes l ≤ 64.
func (r *bitReader) ReadBits(l byte) uint64 {
	read := min(l, r.size)

	ret := r.readBits(read)
	if read == l {
		return ret
	}

	if !r.fill() {
		return 0
	}

	ret |= r.readBits(l-read) << read
	return ret
}

// Read l bits from r, but do not return them.
func (r *bitReader) SkipBits(l byte) {
	read := min(l, r.size)

	if read != r.size {
		r.size -= l
		r.buf >>= l
		return
	}

	if !r.fill() {
		return
	}

	rest := l - read
	r.size -= rest
	r.buf >>= rest
}

func (w *bitWriter) WriteUvarint(x uint64) {
	for x >= 0x80 {
		w.WriteBits(uint64(byte(x)|0x80), 8)
		x >>= 7
	}

	w.WriteBits(uint64(byte(x)), 8)
}

func (r *bitReader) ReadUvarint() uint64 {
	var ret uint64

	for s := 0; s <= 63; s += 7 {
		x := r.ReadBits(7)
		if s == 63 && x > 1 {
			if r.err != nil {
				r.err = errors.New("Uvarint overflow")
			}
			return 0
		}
		ret |= x << s
		more := r.ReadBits(1)

		if more == 0 {
			break
		}
	}

	return ret
}
