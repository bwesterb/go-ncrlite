go-ncrlite
==========

Package to compress a set of positive integers.

**WARNING** The file format is not final yet.


Format
------
The file starts with the **size** of the set as an unsigned varint.

There are two special cases.

1. If the size of the set is zero, the file ends immediately after the size
   (without endmarker.)

2. If the size of the set is one, then the value of that element is encoded
   as an unsigned varint after it and the file ends (without endmarker.)

The values of the set are not encoded directly, but instead their **deltas**
are encoded. The *n*th delta is the difference between the *n*th
and the *n-1*th value, considering the set as a sorted list.

The first delta is special: it's the minimum value of the set plus one
so that a delta is never zero.

For each delta *d*, we consider its **bitlength**. That is the least *l*
such that *2^(l+1) > d*. Note that this is different from the typical
definition of bitlength being one smaller: the length of 1 is 0 and of 4 is 2.

The six least significant bits of the next byte encode the largest bitlength
of any delta that occurs. We assign to that bitlength, and each smaller
bitlength, a canonical Huffman code by encoding the length of each of their
codewords.

The next six bits (that is: the two most significant
bits of the byte used for the largest bitlength, and the four least significant
bits of the byte afterward) encode the length of the codeword for the
zero bitlength.

We continue with the remaining bitlengths in order. If the next bitlength
has a codeword of the same length as the previous codeword, we encode this
with a single bit 1.

Instead, if the codeword is one larger we encode this as first a single bit 0
and then a single bit 1. Together: `0b10`. If it's two larger we repeat
twice: `0b1010`. And so on. If the codeword is one smaller we use `0b00`, and
repeat if the difference is larger.

After having encoded the Huffman code for the bitlengths, we encode
the deltas themselves. First we write the Huffman code for the bitlength.
Then we write the delta with that many bits, without its most significant bit
as it's implied.

Finally, we write the endmarker `0xaa` = `0b10101010`. This allows for simpler
decompression using prefix tables. The remaining high bits in the final byte
are set to zero.
