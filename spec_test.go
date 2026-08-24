package lumen

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestSpecExamplesParse loads every ```lumen code block in LM_SPEC.md into a
// fresh store. The spec documents the parser; this test keeps them from
// drifting apart. Each block must stand alone (fragments get the frames the
// surrounding document declares).
func TestSpecExamplesParse(t *testing.T) {
	data, err := os.ReadFile("LM_SPEC.md")
	if err != nil {
		t.Skip("LM_SPEC.md not present")
	}
	re := regexp.MustCompile("(?s)```lumen\n(.*?)```")
	blocks := re.FindAllStringSubmatch(string(data), -1)
	if len(blocks) == 0 {
		t.Fatal("no lumen code blocks found in LM_SPEC.md")
	}
	for i, m := range blocks {
		src := m[1]
		// Property-level fragments (indented snippets) are not complete files.
		if strings.HasPrefix(strings.TrimSpace(src), "evidence ") {
			continue
		}
		s := NewStore()
		for _, fr := range []string{"reasoning", "empirical", "parametric"} {
			s.RegisterFrame(Frame{Name: fr, Decay: DecayPolicy{Kind: DecayNone}})
		}
		if err := LoadFile(src, s, time.Now()); err != nil {
			t.Errorf("spec block %d does not parse: %v\n---\n%s", i+1, err, src)
		}
	}
}
