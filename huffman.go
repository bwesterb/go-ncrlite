package ncrlite

import (
	"container/heap"
	"errors"
	"fmt"
	"math/bits"
	"slices"
)

// Node in a Huffman tree
type htNode struct {
	value    int // If a leaf, the value: bitlength of the delta (minus one)
	count    int // Cumulative count
	children [2]*htNode
}

// Walk down tree using path. Return node and number of steps taken.
func (n *htNode) Walk(path int) (*htNode, int) {
	i := 0
	cur := n
	var next *htNode

	for {
		next = cur.children[path&1]
		if next == nil {
			return cur, i
		}
		path >>= 1
		i++
		cur = next
	}
}

// Codebook for Huffman code
type htCode []htCodeEntry

type htCodeEntry struct {
	code   int
	length int
}

func (h htCode) Print() {
	for i, entry := range h {
		fmt.Printf("%2d ", i)
		code := entry.code
		for j := 0; j < entry.length; j++ {
			fmt.Printf("%d", code&1)
			code >>= 1
		}
		fmt.Printf("\n")
	}
}

// Pack codebook
func (h htCode) Pack(bw *bitWriter) {
	bw.WriteUvarint(uint64(len(h) - 1))
	bw.WriteUvarint(uint64(h[0].length))

	prev := h[0].length

	for i := 1; i < len(h); i++ {
		l := h[i].length
		absDiff := l - prev
		sign := 1
		if l < prev {
			sign = 0
			absDiff = -absDiff
		}
		for j := 0; j < absDiff; j++ {
			bw.WriteBits(0, 1)
			bw.WriteBits(uint64(sign), 1)
		}
		bw.WriteBits(1, 1)
		prev = l
	}
}

func unpackCodeLengths(br *bitReader) ([]int, error) {
	n := br.ReadUvarint() + 1
	h := make([]int, n)
	h[0] = int(br.ReadUvarint())
	change := 0
	i := 1
	waitingFor := 0

	for {
		next := br.ReadBits(1)
		if next == 1 {
			h[i] = h[i-1] + change
			i++

			if i == int(n) {
				break
			}

			waitingFor = 0
			change = 0
			continue
		}

		waitingFor++
		up := br.ReadBits(1)
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
			value: i,
			count: freq[i],
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
		})
	}

	// There are many equivalent trees; what matters is the length
	// of the code for each value. Find those and find the canonical
	// code for that.
	codeLengths := make([]int, len(freq))

	type nodeDepth struct {
		n     *htNode
		depth int
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

func unpackHuffmanTree(br *bitReader) (*htNode, error) {
	codeLengths, err := unpackCodeLengths(br)
	if err != nil {
		return nil, err
	}

	codebook := canonicalHuffmanCode(codeLengths)

	root := &htNode{}

	for bn, entry := range codebook {
		code := entry.code
		n, d := root.Walk(code)
		code >>= d

		// Create last few nodes
		for j := d; j < entry.length; j++ {
			n.children = [2]*htNode{&htNode{}, &htNode{}}

			n = n.children[code&1]
			code >>= 1
		}

		// Set leaf
		if n.value != 0 || n.children[0] != nil || n.children[1] != nil {
			panic("shoulnd't happen")
		}
		n.value = bn
	}

	return root, nil
}

func canonicalHuffmanCode(codeLengths []int) htCode {
	type valueLength struct {
		value  int
		length int
		code   int
	}

	vls := make([]valueLength, len(codeLengths))
	for i := 0; i < len(codeLengths); i++ {
		vls[i].value = i
		vls[i].length = codeLengths[i]
	}

	slices.SortFunc(vls, func(a, b valueLength) int {
		if a.length != b.length {
			return a.length - b.length
		}
		return a.value - b.value
	})

	prevLength := 0
	code := 0
	ret := make(htCode, len(codeLengths))
	for i := 0; i < len(vls); i++ {
		l := vls[i].length
		if l != prevLength {
			code <<= vls[i].length - prevLength
		}

		vls[i].code = code
		ret[vls[i].value] = htCodeEntry{
			code:   int(bits.Reverse64(uint64(code)) >> (64 - l)),
			length: l,
		}
		prevLength = l
		code++
	}

	return ret
}
