package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// seedModel creates a state root with one entry in state and returns a
// loaded model whose active view is that state.
func seedModel(t *testing.T, state, name string) (model, string) {
	t.Helper()
	root := t.TempDir()
	for _, s := range states {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	body := "[slack] wes: go for it\nhttps://example.com\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, state, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, nil)
	m.view = slices.Index(states, state)
	return m, root
}

// inState returns the states that contain name.
func inState(t *testing.T, root, name string) []string {
	t.Helper()
	var got []string
	for _, s := range states {
		if _, err := os.Stat(filepath.Join(root, s, name)); err == nil {
			got = append(got, s)
		}
	}
	return got
}

func TestHandleKeyMovesOnce(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	tests := []struct {
		key  string
		from string
		want string
	}{
		{"u", "archive", "waiting"},
		{"a", "todo", "archive"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"-from-"+tt.from, func(t *testing.T) {
			m, root := seedModel(t, tt.from, name)
			_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			got := inState(t, root, name)
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("after %q from %s: entry in %v, want [%s]", tt.key, tt.from, got, tt.want)
			}
		})
	}
}
