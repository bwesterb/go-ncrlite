package ncrlite

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
)

type bitReader struct {
	r    *bufio.Reader
	size int
	buf  uint64
	err  error
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
func (r *bitReader) readBits(l int) uint64 {
	ret := r.buf & (uint64(1<<l) - 1)
	r.size -= l
	r.buf >>= l
	return ret
}

func (r *bitReader) ReadBits(l int) uint64 {
	read := min(l, r.size)

	ret := r.readBits(read)
	if read == l {
		return ret
	}

	var buf [8]byte
	n, err := r.r.Read(buf[:])
	if n == 0 {
		r.err = err
		return 0
	}

	// An io.Reader is allowed to use whole of buf as scratch space, so we
	// need to explicitly set to zero.
	for i := n; i < 8; i++ {
		buf[i] = 0
	}

	r.buf = binary.LittleEndian.Uint64(buf[:])
	r.size = 8 * n

	ret |= r.readBits(l-read) << read
	return ret
}

func (w *bitWriter) WriteEliasDelta(x uint64) {
	x++
	N := uint64(bits.Len64(x) - 1)
	L := uint64(bits.Len64(N+1) - 1)
	bs := ((N+1)^(1<<L))<<(L+1) | (x^(1<<N))<<(2*L+1) | (1 << L)
	bl := int(2*L + 1 + N)
	w.WriteBits(bs, bl)
}

func (r *bitReader) ReadEliasDelta() uint64 {
	L := 0
	for r.ReadBits(1) == 0 {
		L++
		if L > 5 {
			return 0xffffffffffffffff
		}
	}
	N := (r.ReadBits(int(L)) | (1 << L)) - 1
	if N > 63 {
		return 0xffffffffffffffff
	}
	return (r.ReadBits(int(N)) | (1 << N)) - 1
}
