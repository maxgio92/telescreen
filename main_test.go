package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestRowAtY(t *testing.T) {
	tests := []struct {
		name                             string
		y, headerLines, start, len, rows int
		want                             int
	}{
		{"first row", 2, 2, 0, 5, 10, 0},
		{"last row", 6, 2, 0, 5, 10, 4},
		{"above the list", 1, 2, 0, 5, 10, -1},
		{"below the last row", 7, 2, 0, 5, 10, -1},
		{"scrolled offset", 3, 2, 4, 20, 10, 5},
		{"below the viewport", 12, 2, 4, 20, 10, -1},
		{"empty list", 2, 2, 0, 0, 10, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowAtY(tt.y, tt.headerLines, tt.start, tt.len, tt.rows); got != tt.want {
				t.Errorf("rowAtY(%d, %d, %d, %d, %d) = %d, want %d",
					tt.y, tt.headerLines, tt.start, tt.len, tt.rows, got, tt.want)
			}
		})
	}
}

func TestViewAtX(t *testing.T) {
	// Widths 7, 6, 9: tabs span [0,7), [9,15), [17,26).
	labels := []string{"1 inbox", "2 todo", "3 waiting"}
	tests := []struct {
		name string
		x    int
		want int
	}{
		{"first tab start", 0, 0},
		{"first tab end", 6, 0},
		{"separator", 7, -1},
		{"separator second column", 8, -1},
		{"second tab", 9, 1},
		{"third tab", 17, 2},
		{"past the last tab", 26, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := viewAtX(tt.x, labels); got != tt.want {
				t.Errorf("viewAtX(%d) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

func TestViewAtXStyledLabels(t *testing.T) {
	styled := []string{
		tabActive.Render("1 inbox"),
		tabInactive.Render("2 todo"),
	}
	if w := lipgloss.Width(styled[0]); w != 7 {
		t.Fatalf("styled label width = %d, want 7", w)
	}
	if got := viewAtX(9, styled); got != 1 {
		t.Errorf("viewAtX(9, styled) = %d, want 1", got)
	}
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
