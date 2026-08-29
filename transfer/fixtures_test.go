package transfer

import (
	"os"
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

func TestTopologyStudyFixturesMatchGenerator(t *testing.T) {
	episodes, err := StudyEpisodes()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join("..", "experiments", "epistemic-transfer", "topology-study", "episodes")
	paths, err := filepath.Glob(filepath.Join(base, "*.lm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != len(episodes) {
		t.Fatalf("fixture count=%d generated=%d", len(paths), len(episodes))
	}
	for _, episode := range episodes {
		want, err := RenderEpisode(episode)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(base, episode.ID+".lm")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("fixture drift: %s; regenerate with cmd/generate-transfer-study", path)
		}
	}
}
