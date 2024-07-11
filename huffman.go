package ncrlite

import (
	"container/heap"
	"slices"
)

// Node in a Huffman tree
type htNode struct {
	value int // If a leaf, the value: bitlength of the delta (minus one)
	count int // Cumulative count
	left  *htNode
	right *htNode
}

// Walk down tree using path. Return node and number of steps taken.
func (n *htNode) Walk(path int) (*htNode, int) {
	i := 0
	cur := n
	var next *htNode

	for {
		if path&1 == 0 {
			next = cur.left
		} else {
			next = cur.right
		}
		if next == nil {
			return cur, i
		}
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

// Pack codebook
func (h htCode) Pack(bw *bitWriter) {
	bw.WriteEliasDelta(uint64(len(h) - 1))
	bw.WriteEliasDelta(uint64(h[0].length))

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
			count: n1.count + n2.count,
			left:  n1,
			right: n2,
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

		if nd.n.left != nil {
			stack = append(
				stack,
				nodeDepth{nd.n.left, nd.depth + 1},
				nodeDepth{nd.n.right, nd.depth + 1},
			)
			continue
		}

		codeLengths[nd.n.value] = nd.depth
	}

	return canonicalHuffmanCode(codeLengths)
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
			code:   code,
			length: l,
		}
		prevLength = l
		code++

		// // Walk down partial tree now
		// n, d := root.Walk(code)
		// code >>= d

		// // Create last few nodes
		// for j := d; j < vls[i].length; j++ {
		// 	n.left = &htNode{}
		// 	n.right = &htNode{}

		// 	if code&1 == 0 {
		// 		n = n.left
		// 	} else {
		// 		n = n.right
		// 	}
		// }

		// // Set leaf
		// if n.value != 0 || n.left != nil || n.right != nil {
		// 	panic("shoulnd't happen")
		// }
		// n.value = vls[i].value
	}

	return ret
}
