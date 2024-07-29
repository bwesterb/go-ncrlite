package ncrlite

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"slices"
)

// Prefix table used to represent Huffman tree during decompression:
// eight layers are combined into one for fast decompression.
type htLut []htLutEntry

type htLutEntry struct {
	value byte // If a leaf, the value: bitlength of the delta (minus one)

	// If a leaf, the number of bits to skip in the last byte. Zero indicates
	// this is a node.
	skip byte

	// If a node, offset into the table to find children.
	next int
}

// Node in a Huffman tree when constructed from frequences
type htNode struct {
	value    byte // If a leaf, the value: bitlength of the delta (minus one)
	count    int  // Cumulative count
	children [2]*htNode
	depth    int
}

// Codebook for Huffman code
type htCode []htCodeEntry

type htCodeEntry struct {
	code   uint64
	length byte
}

func (h htLut) Print(w io.Writer) {
	for i := 0; i < len(h); i += 256 {
		fmt.Fprintf(w, "offset %d:", i)

		for code := 0; code < 256; code++ {
			for j := 0; j < int(8); j++ {
				fmt.Fprintf(w, "%d", (code>>j)&1)
			}

			fmt.Fprintf(w, " value=%d skip=%d next=%d\n", h[i+code].value, h[i+code].skip, h[i+code].next)
		}
	}
}

func (h htCode) Print(w io.Writer) {
	for i, entry := range h {
		fmt.Fprintf(w, "%2d ", i)
		code := entry.code
		for j := 0; j < int(entry.length); j++ {
			fmt.Fprintf(w, "%d", code&1)
			code >>= 1
		}
		fmt.Fprintf(w, "\n")
	}
}

// Pack codebook
func (h htCode) Pack(bw *bitWriter) {
	bw.WriteBits(uint64(len(h)-1), 6)
	bw.WriteBits(uint64(h[0].length), 6)

	prev := h[0].length

	for i := 1; i < len(h); i++ {
		l := h[i].length
		absDiff := l - prev
		sign := 1
		if l < prev {
			sign = 0
			absDiff = -absDiff
		}
		for j := 0; j < int(absDiff); j++ {
			bw.WriteBits(0, 1)
			bw.WriteBits(uint64(sign), 1)
		}
		bw.WriteBits(1, 1)
		prev = l
	}
}

func unpackCodeLengths(br *bitReader, l io.Writer) ([]byte, error) {
	size := 12
	n := br.ReadBits(6) + 1
	h := make([]byte, n)
	h[0] = byte(br.ReadBits(6))
	if l != nil {
		fmt.Fprintf(l, "max bitlength        %d\n", n-1)
		fmt.Fprintf(l, "codelength h[0]      %d\n", h[0])

		defer func() {
			fmt.Fprintf(l, "dictionary size      %db\n", size)
		}()
	}

	if n == 1 {
		return h, br.Err()
	}

	change := int8(0)
	i := 1
	waitingFor := 0

	for {
		size++
		next := br.ReadBit()
		if next == 1 {
			h[i] = byte(int8(h[i-1]) + change)
			i++

			if i == int(n) {
				break
			}

			waitingFor = 0
			change = 0
			continue
		}

		waitingFor++
		size++
		up := br.ReadBit()
		if up == 1 {
			change++
		} else {
			change--
		}

		if waitingFor > int(n) {
			return nil, errors.New("invalid codelength in Huffman table")
		}
	}

	return h, br.Err()
}

// Priority queue to find nodes with lowest count
type htHeap []*htNode

func (h htHeap) Len() int { return len(h) }
func (h htHeap) Less(i, j int) bool {
	if h[i].count == h[j].count {
		return h[i].depth < h[j].depth
	}
	return h[i].count < h[j].count
}

func (h htHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *htHeap) Push(x any) {
	node := x.(*htNode)
	*h = append(*h, node)
}

func (h *htHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return item
}

// Create a Huffman code for the given frequency table
func buildHuffmanCode(freq []int) htCode {
	h := make(htHeap, len(freq))

	for i := 0; i < len(freq); i++ {
		h[i] = &htNode{
			value: byte(i),
			count: freq[i],
			depth: 0,
		}
	}

	heap.Init(&h)

	// Build the tree: combine the two subtrees with the shortest count
	// repetitively.
	for len(h) > 1 {
		n1 := heap.Pop(&h).(*htNode)
		n2 := heap.Pop(&h).(*htNode)

		heap.Push(&h, &htNode{
			count:    n1.count + n2.count,
			children: [2]*htNode{n1, n2},
			depth:    max(n1.depth, n2.depth) + 1,
		})
	}

	// There are many equivalent trees; what matters is the length
	// of the code for each value. Find those and find the canonical
	// code for that.
	codeLengths := make([]byte, len(freq))

	type nodeDepth struct {
		n     *htNode
		depth byte
	}

	stack := []nodeDepth{{n: h[0], depth: 0}}
	h = nil

	for len(stack) > 0 {
		nd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if nd.n.children[0] != nil {
			stack = append(
				stack,
				nodeDepth{nd.n.children[0], nd.depth + 1},
				nodeDepth{nd.n.children[1], nd.depth + 1},
			)
			continue
		}

		codeLengths[nd.n.value] = nd.depth
	}

	codebook := canonicalHuffmanCode(codeLengths)
	return codebook
}

func unpackHuffmanTree(br *bitReader, l io.Writer) (htLut, error) {
	codeLengths, err := unpackCodeLengths(br, l)
	if err != nil {
		return nil, err
	}

	// Special case: if there
	if len(codeLengths) == 1 {
		if l != nil {
			fmt.Fprintf(l, "\nTrivial codebook: only zero bitlength deltas\n\n")
		}
		return nil, nil
	}

	codebook := canonicalHuffmanCode(codeLengths)

	if l != nil {
		fmt.Fprintf(l, "\nCodebook bitlengths:\n")
		codebook.Print(l)
	}

	// Build the binary tree before building the prefix table
	root := &htNode{}

	for bn, entry := range codebook {
		code := entry.code

		// Walk down the existing tree
		node := root
		d := 0
		for {
			next := node.children[code&1]

			if next == nil {
				break
			}

			d++
			code >>= 1
			node = next
		}

		// Now create the new nodes
		for j := d; j < int(entry.length); j++ {
			node.children[code&1] = &htNode{}
			node = node.children[code&1]
			code >>= 1
		}

		node.value = byte(bn)
	}

	// Build the prefix table
	lut := make(htLut, 256)

	type todoEntry struct {
		node   *htNode
		offset int // in htLut
	}

	todo := []todoEntry{{root, 0}}

	for len(todo) > 0 {
		cur := todo[len(todo)-1]
		todo = todo[:len(todo)-1]

		for code := 0; code < 256; code++ {
			node := cur.node
			skip := 0
			for ; skip < 8; skip++ {
				next := node.children[(code>>skip)&1]
				if next == nil {
					break
				}
				node = next
			}

			if node.children[0] == nil {
				lut[cur.offset+code].skip = byte(skip)
				lut[cur.offset+code].value = node.value
				continue
			}

			lut[cur.offset+code].skip = 0
			lut[cur.offset+code].next = len(lut)
			todo = append(todo, todoEntry{
				node:   node,
				offset: len(lut),
			})

			for i := 0; i < 256; i++ {
				lut = append(lut, htLutEntry{})
			}
		}
	}

	return lut, nil
}

func canonicalHuffmanCode(codeLengths []byte) htCode {
	type valueLength struct {
		value  byte
		length byte
		code   uint64
	}

	vls := make([]valueLength, len(codeLengths))
	for i := 0; i < len(codeLengths); i++ {
		vls[i].value = byte(i)
		vls[i].length = codeLengths[i]
	}

	slices.SortFunc(vls, func(a, b valueLength) int {
		if a.length != b.length {
			return int(a.length) - int(b.length)
		}
		return int(a.value) - int(b.value)
	})

	prevLength := byte(0)
	code := uint64(0)
	ret := make(htCode, len(codeLengths))
	for i := 0; i < len(vls); i++ {
		l := vls[i].length
		if l != prevLength {
			code <<= vls[i].length - prevLength
		}

		vls[i].code = code
		ret[vls[i].value] = htCodeEntry{
			code:   bits.Reverse64(code) >> (64 - l),
			length: l,
		}
		prevLength = l
		code++
	}

	return ret
}
