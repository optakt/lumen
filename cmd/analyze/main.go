// Command analyze reads a natural language text file and extracts belief
// candidates, emitting a .lm file suitable for loading into a Lumen store.
//
// Usage: analyze <file.txt> [--out file.lm] [--frame empirical]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lumen "github.com/optakt/lumen"
)

func main() {
	out   := flag.String("out", "", "output .lm file (default: stdout)")
	frame := flag.String("frame", "", "override suggested frame")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: analyze [--out file.lm] [--frame FRAME] <file.txt>")
		os.Exit(1)
	}

	src, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}

	analysis := lumen.AnalyzeText(string(src))
	if *frame != "" {
		analysis.Frame = *frame
	}

	name := filepath.Base(flag.Arg(0))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	lmContent := analysis.ToLumenFile(name)

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "Extracted: %d records, %d beliefs, %d entities\n",
		len(analysis.Records), len(analysis.Beliefs), len(analysis.Entities))
	fmt.Fprintf(os.Stderr, "Suggested frame: %s\n", analysis.Frame)
	if len(analysis.Entities) > 0 && len(analysis.Entities) <= 10 {
		fmt.Fprintf(os.Stderr, "Entities: %s\n", strings.Join(analysis.Entities, ", "))
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(lmContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Written to %s\n", *out)
	} else {
		fmt.Print(lmContent)
	}
}
