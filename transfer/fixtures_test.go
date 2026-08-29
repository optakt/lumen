package transfer

import (
	"path/filepath"
	"testing"
)

func TestPilotEpisodesParse(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "experiments", "epistemic-transfer", "pilot", "episodes", "*.lm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 6 {
		t.Fatalf("found %d pilot episodes, want 6", len(paths))
	}
	families := map[string]int{}
	variants := map[string]int{}
	for _, path := range paths {
		episode, err := ParseFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		families[episode.Family]++
		variants[episode.Variant]++
	}
	if len(families) != 3 || variants["a"] != 3 || variants["b"] != 3 {
		t.Fatalf("families=%v variants=%v", families, variants)
	}
}
