package main

import (
	"fmt"
	"os"
	lumen "github.com/optakt/lumen"
)

func main() {
	src := "decay: exponential halflife: 1h\nat: \"2026-01-01T14:00:00Z\""
	if len(os.Args) > 1 {
		src = os.Args[1]
	}
	tokens, err := lumen.Tokenize(src)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, t := range tokens {
		fmt.Printf("  %s\n", t)
	}
}
