package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStudyEpisodesBalancedAndRoundTrip(t *testing.T) {
	episodes, err := StudyEpisodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 40 {
		t.Fatalf("episodes = %d, want 40", len(episodes))
	}
	families := map[string]int{}
	topologies := map[string]int{}
	variants := map[string]int{}
	seen := map[string]bool{}
	for _, episode := range episodes {
		if seen[episode.ID] {
			t.Fatalf("duplicate episode %s", episode.ID)
		}
		seen[episode.ID] = true
		families[episode.Family]++
		topologies[episode.Topology]++
		variants[episode.Variant]++

		text, err := RenderEpisode(episode)
		if err != nil {
			t.Fatalf("render %s: %v", episode.ID, err)
		}
		path := filepath.Join(t.TempDir(), episode.ID+".lm")
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseFile(path)
		if err != nil {
			t.Fatalf("parse rendered %s: %v", episode.ID, err)
		}
		if parsed.ID != episode.ID || parsed.Topology != episode.Topology || len(parsed.Steps) != len(episode.Steps) {
			t.Fatalf("round-trip mismatch for %s", episode.ID)
		}
	}
	if len(families) != 5 || len(topologies) != 4 || len(variants) != 2 {
		t.Fatalf("families=%v topologies=%v variants=%v", families, topologies, variants)
	}
	for family, count := range families {
		if count != 8 {
			t.Fatalf("family %s count=%d", family, count)
		}
	}
	for topology, count := range topologies {
		if count != 10 {
			t.Fatalf("topology %s count=%d", topology, count)
		}
	}
	for variant, count := range variants {
		if count != 20 {
			t.Fatalf("variant %s count=%d", variant, count)
		}
	}
}

func TestStudyReferenceIntervalsRemainValid(t *testing.T) {
	episodes, err := StudyEpisodes()
	if err != nil {
		t.Fatal(err)
	}
	for _, episode := range episodes {
		for _, step := range episode.Steps {
			if !step.Reference.Belief.Valid() {
				t.Fatalf("%s/%s invalid belief %#v", episode.ID, step.ID, step.Reference.Belief)
			}
			if step.Reference.HistoricalBelief != nil && !step.Reference.HistoricalBelief.Valid() {
				t.Fatalf("%s/%s invalid historical belief", episode.ID, step.ID)
			}
		}
	}
}
