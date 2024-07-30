go-ncrlite
==========

*ncrlite* is a simple and fast compression format specifically designed to compress an unordered
set of positive integers (below 2⁶⁴).
This repository contains a [Go package](https://pkg.go.dev/github.com/bwesterb/go-ncrlite#Compress)
that implements it and a commandline tool.

**Warning.** The file format is not yet final.

Performance
-----------

*ncrlite* achieves smaller compressed sizes than general-purpose compressors.

| dataset | description | CSV | ncrlite | gzip -9 | xz -9 |
| --- | --- | --- | --- | --- | --- |
| [le.csv](https://westerbaan.name/~bas/ncrlite/le.csv.ncrlite) | Sequence numbers of Let's Encrypt certificates revoked on July 18th, 2024 | 4.8MB | 706kB | 1.7MB | 900kB |
| [primes.csv](https://westerbaan.name/~bas/ncrlite/primes.csv.ncrlite) | First million prime numbers | 8.2MB | 674kB | 2.4MB | 941kB |
| [sigs.csv](https://westerbaan.name/~bas/ncrlite/sigs.csv.ncrlite) | List of the 9 signature algorithms supported by Chrome 126 | 44B | 16B | 58B | 96B |

Commandline tool
----------------

### Installation

Install [Go](https://go.dev/doc/install) and run

```
$ go install github.com/bwesterb/go-ncrlite/cmd/ncrlite@latest
```

Now you can use `ncrlite`.

### Basic operation

`ncrlite` takes as input a textfile with a positive number on each line.

```
$ cat dunbar
5
15
35
150
500
1500
```

To compress simply run:

```
$ ncrlite dunbar
```

This will create `dunbar.ncrlite` and remove `dunbar`.

The input file does not have to be sorted (numerically). If it is not, `ncrlite` will sort the input first, which is slower.

To decompress, run:

```
$ ncrlite -d dunbar.ncrlite
```

This will create `dunbar` and remove `dunbar.ncrlite`. The output file is always sorted.

### Other formats

At the moment, the `ncrlite` commandline tool only supports the simple text format.
[Reach out](https://github.com/bwesterb/go-ncrlite/issues/1) if another is useful.

### Other flags

`ncrlite` supports several familiar flags.

```
  -f, --force
    	overwrite output
  -k, --keep
    	keep (don't delete) input file
  -c, --stdout
    	write to stdout; implies -k
```

Without specifying a filename (or using `-`),
`ncrlite` will read from `stdin` and write to `stdout`.

### Inspect compressed file

With `-i` we can inspect a compressed file:

```
$ ncrlite -i le.csv.ncrlite 
max bitlength        14
codelength h[0]      9
dictionary size      56b

Codebook bitlengths:
 0 111111110
 1 11111110
 2 1111100
 3 1111101
 4 11110
 5 1100
 6 1101
 7 100
 8 00
 9 01
10 101
11 1110
12 1111110
13 1111111110
14 1111111111

Maximum value    (N)  382584265
Number of values (k)  512652
Theoretical best avg  703953.8B
Overhead              0.4%
```

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
