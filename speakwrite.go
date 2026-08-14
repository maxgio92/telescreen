// The s dictation key: the consumer writes a pre-filled intent file,
// suspends into the editor, and submits it by renaming the temporary
// file into recdep/intents/. A separate runner consumes intents;
// nothing here calls the network.

package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// actionRule maps an entry shape to a speakwrite action. Empty fields
// match anything; the first matching rule in actionRules wins.
type actionRule struct {
	nameSubstring string // matches when the filename contains this
	source        string // matches when the source equals this
	whoSuffix     string // matches when who ends with this
	action        string
}

// actionRules is the v1 source-action map, in match order. It stays a
// data table so a config file can override it later. Review requests
// match on the filename slug: minitrue tags every GitHub entry [github]
// in the header, so the slug is the only discriminator.
var actionRules = []actionRule{
	{source: "github-review-requested", action: "review"},
	{nameSubstring: "-review-requested-", source: "github", action: "review"},
	{source: "github", whoSuffix: "[bot]", action: "vet-findings"},
	{source: "github", action: "pr-reply"},
	{source: "slack", action: "slack-reply"},
	{source: "linear", action: "linear-comment"},
}

// actionFor returns the speakwrite action for an entry, "respond" when
// no rule matches.
func actionFor(e entry) string {
	for _, r := range actionRules {
		if r.nameSubstring != "" && !strings.Contains(e.name, r.nameSubstring) {
			continue
		}
		if r.source != "" && e.source != r.source {
			continue
		}
		if r.whoSuffix != "" && !strings.HasSuffix(e.who, r.whoSuffix) {
			continue
		}
		return r.action
	}
	return "respond"
}

// dictatedGuidance extracts the guidance text of the last dictated
// section: the lines after the last "--- dictated" marker line up to the
// next marker or the end, trailing blank lines dropped. Empty when the
// body has no dictated section.
func dictatedGuidance(body string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if markerKind(line) == "dictated" {
			start = i + 1
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if markerKind(lines[i]) != "" {
			end = i
			break
		}
	}
	g := lines[start:end]
	for len(g) > 0 && strings.TrimSpace(g[len(g)-1]) == "" {
		g = g[:len(g)-1]
	}
	return strings.Join(g, "\n")
}

// renderIntent renders the intent file per RECDEP.md: the entry path
// line, the action line, and a guidance section pre-filled with the
// previous guidance when re-dictating.
func renderIntent(path string, e entry, guidance string) string {
	return "entry " + path + "\naction " + actionFor(e) + "\n\nguidance:\n" + guidance + "\n"
}

// intentGuidance extracts the text under an intent's "guidance:" line.
func intentGuidance(s string) string {
	_, after, ok := strings.Cut(s, "\nguidance:\n")
	if !ok {
		return ""
	}
	return strings.TrimRight(after, "\n")
}

// guidanceFor returns the re-dictation pre-fill: a pending intent's
// guidance when one exists (the runner has not consumed it yet), else
// the body's last dictated section.
func guidanceFor(root string, e entry) string {
	if b, err := os.ReadFile(filepath.Join(root, intentsDir, e.name+".intent")); err == nil {
		if g := intentGuidance(string(b)); g != "" {
			return g
		}
	}
	return dictatedGuidance(e.body)
}

// dictationSubmits reports whether an editor round submits the intent:
// the editor exited zero and the file still has content. An emptied or
// deleted file (nil content) cancels; empty guidance alone still submits
// because the action's defaults apply.
func dictationSubmits(content []byte, err error) bool {
	return err == nil && len(bytes.TrimSpace(content)) > 0
}

// editorDoneMsg reports the editor exiting after a dictation on the
// named entry.
type editorDoneMsg struct {
	name string
	err  error
}

// dictate handles the s key: in inbox, todo, and waiting it writes a
// pre-filled draft intent and suspends into the editor. Archive and the
// memory hole hold closed or destroyed records; dictation is a no-op.
func (m model) dictate() (tea.Model, tea.Cmd) {
	if m.view >= len(states) || states[m.view] == "archive" {
		return m, nil
	}
	e, ok := m.selected()
	if !ok {
		return m, nil
	}
	entryPath := filepath.Join(m.root, states[m.view], e.name)
	tmp := filepath.Join(m.root, intentsDir, e.name+".intent.tmp")
	if err := os.WriteFile(tmp, []byte(renderIntent(entryPath, e, guidanceFor(m.root, e))), 0o644); err != nil {
		m.status = err.Error()
		return m, nil
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// The convention allows arguments in the value ("code -w").
	args := strings.Fields(editor)
	args = append(args, tmp)
	name := e.name
	return m, tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		return editorDoneMsg{name: name, err: err}
	})
}

// finishDictation submits or cancels after the editor exits: a nonzero
// exit or an emptied file removes the draft, anything else renames it to
// the final intent name (the atomic submit the runner watches for).
func (m *model) finishDictation(msg editorDoneMsg) {
	tmp := filepath.Join(m.root, intentsDir, msg.name+".intent.tmp")
	content, err := os.ReadFile(tmp)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// A permissions or IO failure is not a cancellation; keep the
		// dictation on disk and surface the error.
		m.status = err.Error()
		return
	}
	if !dictationSubmits(content, msg.err) {
		_ = os.Remove(tmp)
		m.status = "dictation cancelled"
		return
	}
	if err := os.Rename(tmp, filepath.Join(m.root, intentsDir, msg.name+".intent")); err != nil {
		m.status = err.Error()
		return
	}
	m.reload()
	m.status = "dictated " + msg.name
}
