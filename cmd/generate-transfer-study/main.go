// generate-transfer-study writes the deterministic topology-held-out corpus.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/optakt/lumen/transfer"
)

func main() {
	out := flag.String("out", "episodes", "output directory")
	flag.Parse()

	episodes, err := transfer.StudyEpisodes()
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		panic(err)
	}
	for _, episode := range episodes {
		text, err := transfer.RenderEpisode(episode)
		if err != nil {
			panic(err)
		}
		path := filepath.Join(*out, episode.ID+".lm")
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			panic(err)
		}
	}
	fmt.Printf("wrote %d episodes to %s\n", len(episodes), *out)
}
