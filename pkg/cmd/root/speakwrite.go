// The speakwrite keys. s dictates: the consumer writes a pre-filled
// intent file, suspends into the editor, and submits it by renaming the
// temporary file into recdep/intents/. p p approves publication by
// writing a .publish file next to the intents. D discards a draft by
// appending the discarded marker. A separate runner consumes the intent
// and approval files; nothing here calls the network.

package root

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/maxgio92/telescreen/internal/config"
	"github.com/maxgio92/telescreen/internal/publish"
	"github.com/maxgio92/telescreen/internal/recdep"
)

// actionRule maps an entry shape to a speakwrite action. Empty fields
// match anything; the first matching rule in actionRules wins.
type actionRule struct {
	nameSubstring string // matches when the filename contains this
	source        string // matches when the source equals this
	whoSuffix     string // matches when who ends with this
	urlPrefix     string // matches when the URL starts with this
	author        string // matches when who equals this
	action        string
	guidance      string // travels with the action into the intent
}

// actionRules is the source-action map, in match order. It stays a data
// table so telescreen.yaml can replace it (applyConfig). Review requests
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

// applyConfig replaces the built-in action map with the configured one.
// An empty list keeps the built-ins: override is all or nothing, never
// a merge.
func applyConfig(c config.Config) {
	if len(c.Speakwrite.Actions) == 0 {
		return
	}
	rules := make([]actionRule, len(c.Speakwrite.Actions))
	for i, a := range c.Speakwrite.Actions {
		rules[i] = actionRule{
			nameSubstring: a.NameContains,
			source:        a.Source,
			whoSuffix:     a.WhoSuffix,
			urlPrefix:     a.URLPrefix,
			author:        a.Author,
			action:        a.Action,
			guidance:      a.Guidance,
		}
	}
	actionRules = rules
}

// actionFor returns the speakwrite action for an entry and the matching
// rule's guidance, "respond" with no guidance when no rule matches.
func actionFor(e recdep.Entry) (action, guidance string) {
	for _, r := range actionRules {
		if r.nameSubstring != "" && !strings.Contains(e.Name, r.nameSubstring) {
			continue
		}
		if r.source != "" && e.Source != r.source {
			continue
		}
		if r.whoSuffix != "" && !strings.HasSuffix(e.Who, r.whoSuffix) {
			continue
		}
		if r.urlPrefix != "" && !strings.HasPrefix(e.URL, r.urlPrefix) {
			continue
		}
		if r.author != "" && e.Who != r.author {
			continue
		}
		return r.action, r.guidance
	}
	return "respond", ""
}

// composeGuidance prepends the rule guidance to the dictated stance so
// the live dictation reads as refining it. A stance already starting
// with the rule guidance as its own line (a re-dictation pre-fill) is
// kept as is; the guard is line-level so a fresh stance that merely
// begins with the same words still gets the prefix. A pre-fill carrying
// an older rule text after a config edit keeps both, newest first.
func composeGuidance(rule, typed string) string {
	switch {
	case rule == "":
		return typed
	case typed == "":
		return rule
	case typed == rule || strings.HasPrefix(typed, rule+"\n"):
		return typed
	}
	return rule + "\n" + typed
}

// renderIntent renders the intent file per docs/contracts/recdep.md: the entry path
// line, the action line, and a guidance section holding the matched
// rule's guidance followed by the previous guidance when re-dictating.
func renderIntent(path string, e recdep.Entry, guidance string) string {
	action, rule := actionFor(e)
	return "entry " + path + "\naction " + action + "\n\nguidance:\n" + composeGuidance(rule, guidance) + "\n"
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
func guidanceFor(root string, e recdep.Entry) string {
	if b, err := os.ReadFile(filepath.Join(root, recdep.IntentsDir, e.Name+".intent")); err == nil {
		if g := intentGuidance(string(b)); g != "" {
			return g
		}
	}
	g, _ := recdep.LastSection(e.Body, "dictated")
	return g
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

// dictate handles the s key: in tube, desk, and upsub it writes a
// pre-filled draft intent and suspends into the editor. Files and the
// memory hole hold closed or destroyed records; dictation is a no-op.
func (m model) dictate() (tea.Model, tea.Cmd) {
	if m.view >= len(recdep.States) || recdep.States[m.view] == "files" {
		return m, nil
	}
	e, ok := m.selected()
	if !ok {
		return m, nil
	}
	entryPath := filepath.Join(m.root, recdep.States[m.view], e.Name)
	tmp := filepath.Join(m.root, recdep.IntentsDir, e.Name+".intent.tmp")
	if err := os.WriteFile(tmp, []byte(renderIntent(entryPath, e, guidanceFor(m.root, e))), 0o600); err != nil {
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
	name := e.Name
	return m, tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		return editorDoneMsg{name: name, err: err}
	})
}

// publish handles the p key: on a drafted entry the first p arms it and
// a second consecutive p writes the publish approval into recdep/intents/.
// The runner posts; the TUI stays offline. Publishing is double-keyed
// like incinerate because it is outward-facing the way incineration is
// destructive.
func (m *model) publish(armed string) {
	if m.view >= len(recdep.States) {
		return
	}
	e, ok := m.selected()
	if !ok || e.Mark != "draft" {
		return
	}
	// The gate reads the same publisher table the actor dispatches on
	// (internal/publish), so the view never approves what the actor
	// would refuse. Match parses the URL only; the TUI stays offline.
	if _, ok := publish.Match(e.URL); !ok {
		m.status = "no publisher for this record's target; y copies the url"
		return
	}
	if armed != e.Name {
		m.pubArmed = e.Name
		m.status = "approve posting to " + e.URL + ": press p again"
		return
	}
	entryPath := filepath.Join(m.root, recdep.States[m.view], e.Name)
	approval := filepath.Join(m.root, recdep.IntentsDir, e.Name+".publish")
	// tmp plus rename, like the dictation submit: the runner globs the
	// final name and must never see a half-written approval.
	if err := os.WriteFile(approval+".tmp", []byte("entry "+entryPath+"\n"), 0o600); err != nil {
		m.status = err.Error()
		return
	}
	if err := os.Rename(approval+".tmp", approval); err != nil {
		m.status = err.Error()
		return
	}
	m.status = "approved " + e.Name
}

// discard handles the D key: on a drafted entry it appends the
// discarded marker, the one consumer content-write docs/contracts/recdep.md sanctions.
// The draft stays in the record but stops rendering as actionable.
func (m *model) discard() {
	if m.view >= len(recdep.States) {
		return
	}
	e, ok := m.selected()
	if !ok || e.Mark != "draft" {
		return
	}
	entryPath := filepath.Join(m.root, recdep.States[m.view], e.Name)
	marker := "--- discarded " + time.Now().UTC().Format(time.RFC3339) + "\n"
	if err := recdep.AppendMarker(entryPath, marker); err != nil {
		m.status = err.Error()
		return
	}
	// Revoke a pending publish approval: the runner would otherwise
	// still post the withdrawn draft.
	if err := os.Remove(filepath.Join(m.root, recdep.IntentsDir, e.Name+".publish")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		m.status = err.Error()
		return
	}
	m.reload()
	m.status = "draft discarded"
}

// finishDictation submits or cancels after the editor exits: a nonzero
// exit or an emptied file removes the draft, anything else renames it to
// the final intent name (the atomic submit the runner watches for).
func (m *model) finishDictation(msg editorDoneMsg) {
	tmp := filepath.Join(m.root, recdep.IntentsDir, msg.name+".intent.tmp")
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
	if err := os.Rename(tmp, filepath.Join(m.root, recdep.IntentsDir, msg.name+".intent")); err != nil {
		m.status = err.Error()
		return
	}
	m.reload()
	m.status = "dictated " + msg.name
}
