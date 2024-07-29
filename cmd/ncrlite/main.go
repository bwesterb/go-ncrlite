package main

import (
	"github.com/bwesterb/go-ncrlite"

	"rsc.io/getopt"

	"golang.org/x/term"

	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

var (
	// Flags

	decompress = flag.Bool("decompress", false, "specify to decompress")
	info       = flag.Bool("info", false, "specify to print info on compressed file")
	keep       = flag.Bool("keep", false, "keep (don't delete) input file")
	toStdout   = flag.Bool("stdout", false, "write to stdout; implies -k")
	force      = flag.Bool("force", false, "overwrite output")

	// State
	inPath  string
	inFile  *os.File
	outPath string
	outFile *os.File
)

// Approximates lg n! using Stirling's approximation
func lgfac(n uint64) float64 {
	fn := float64(n)
	return math.Log2(2*math.Pi*fn)/2 + fn*math.Log2(fn) - fn*math.Log2(math.E)
}

// Approximates lg n choose k.
func lgncr(n, k uint64) float64 {
	return lgfac(n) - lgfac(k) - lgfac(n-k)
}

const extension = ".ncrlite"

func doDecompress() int {
	var w *bufio.Writer

	if outFile == nil {
		w = bufio.NewWriter(io.Discard)
	} else {
		w = bufio.NewWriter(outFile)
	}

	r := bufio.NewReader(inFile)
	var l io.Writer

	if *info {
		l = os.Stdout
	}

	d, err := ncrlite.NewDecompressorWithLogging(r, l)

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", inPath, err)
		return 8
	}

	var (
		xs     [512]uint64
		toRead []uint64
	)

	// For statistics when in info mode
	k := d.Remaining()

	for d.Remaining() > 0 {
		toRead = xs[:min(len(xs), int(d.Remaining()))]
		err = d.Read(toRead)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", inPath, err)
			return 9
		}

		for _, x := range toRead {
			_, err = fmt.Fprintf(w, "%d\n", x)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", outPath, err)
				return 10
			}
		}
	}

	if l != nil {
		N := uint64(0)

		if k != 0 {
			N = toRead[len(toRead)-1] + 1
		}

		shannon := lgncr(N, k) / 8

		fmt.Fprintf(l, "Maximum value    (N)  %d\n", N)
		fmt.Fprintf(l, "Number of values (k)  %d\n", k)
		fmt.Fprintf(l, "Theoretical best avg  %.1fB\n", shannon)
		fmt.Fprintf(
			l,
			"Overhead              %.1f%%\n",
			100*(float64(d.BytesRead())/float64(shannon)-1.0),
		)
	}

	err = w.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", outPath, err)
		return 10
	}

	return 0
}

func doCompress() int {
	var err error
	scanner := bufio.NewScanner(inFile)

	var prev uint64
	sorted := true
	line := 0
	xs := []uint64{}

	for scanner.Scan() {
		cur, err := strconv.ParseUint(scanner.Text(), 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s:%d %v\n", inPath, line, err)
			return 5
		}
		if line != 0 && cur == prev {
			fmt.Fprintf(os.Stderr, "%s:%d dulpicate value %d\n", inPath, line, cur)
			return 6
		}
		if cur < prev {
			sorted = false
		}
		line++
		xs = append(xs, cur)
		prev = cur
	}

	w := bufio.NewWriter(outFile)

	if sorted {
		err = ncrlite.CompressSorted(w, xs)
	} else {
		fmt.Fprintf(os.Stderr, "%s: input unsorted\n", inPath)
		err = ncrlite.Compress(w, xs)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", outPath, err)
		return 7
	}

	err = w.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: write: %v\n", outPath, err)
		return 7
	}

	return 0
}

func do() int {
	var (
		err  error
		code int
	)

	if len(flag.Args()) > 1 {
		fmt.Fprintf(os.Stderr, "too many arguments\n")
		return 2
	}

	if len(flag.Args()) == 0 {
		inPath = "-"
	} else {
		inPath = flag.Args()[0]
	}

	closeInput := false
	closeOutput := false

	defer func() {
		if closeInput {
			inFile.Close()
		}

		if closeOutput {
			outFile.Close()

			if code != 0 {
				os.Remove(outPath)
			}
		}
	}()

	if inPath == "-" {
		inFile = os.Stdin
		closeInput = false
	} else {
		if _, err := os.Stat(inPath); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", inPath, err)
			return 1
		}

		inFile, err = os.Open(inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", inPath, err)
			return 3
		}
		closeInput = true
	}

	if inPath == "-" {
		outPath = "-"
	} else {
		if *toStdout {
			outPath = "-"
		} else if *decompress {
			if strings.HasSuffix(inPath, extension) {
				outPath = inPath[:len(inPath)-len(extension)]
			} else {
				outPath = inPath + ".out"
				fmt.Fprintf(
					os.Stderr,
					"%s: Unknown extension, writing to %s\n",
					inPath,
					outPath,
				)
			}
		} else if !*info {
			outPath = inPath + extension
		}
	}

	if *info && !*decompress {
		outFile = nil
	} else if outPath == "-" {
		outFile = os.Stdout

		if term.IsTerminal(int(os.Stdout.Fd())) && !*decompress && !*info {
			fmt.Fprintf(os.Stderr, "ncrlite: I'm not writing compressed data to stdout\n")
			return 13
		}
	} else if !*info {
		if _, err := os.Stat(outPath); !*force && err == nil {
			fmt.Fprintf(os.Stderr, "%s: already exists\n", outPath)
			return 11
		}

		outFile, err = os.Create(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: create: %v\n", outPath, err)
			return 4
		}

		closeOutput = true
	}

	if *decompress || *info {
		code = doDecompress()
	} else {
		code = doCompress()
	}

	if closeInput {
		closeInput = false
		inFile.Close()

		if !*keep && !*toStdout && code == 0 && !*info {
			err = os.Remove(inPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: unlink: %v\n", inPath, err)
				return 2
			}
		}
	}

	return code
}

func main() {
	getopt.Alias("d", "decompress")
	getopt.Alias("k", "keep")
	getopt.Alias("c", "stdout")
	getopt.Alias("f", "force")
	getopt.Alias("i", "info")

	// Work around https://github.com/rsc/getopt/issues/3
	err := getopt.CommandLine.Parse(os.Args[1:])
	if err != nil {
		os.Exit(12)
	}

	ret := do()
	os.Exit(ret)
}
