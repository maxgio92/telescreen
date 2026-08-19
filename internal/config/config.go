// Package config loads the user configuration for the telescreen:
// one component-keyed file at <user config dir>/telescreen.yaml that
// configures the whole pipeline. A missing file means defaults; when
// only the retired <user config dir>/recdep/config.yaml exists, its
// actions load as speakwrite.actions so existing setups keep working.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// Action is one row of the dictation action map. The match fields are
// optional and empty fields match anything; Action names the speakwrite
// action a matching entry dictates.
type Action struct {
	// Source matches when it equals the entry's source tag.
	Source string `yaml:"source"`
	// NameContains matches when the entry filename contains it.
	NameContains string `yaml:"name_contains"`
	// WhoSuffix matches when the entry author ends with it.
	WhoSuffix string `yaml:"who_suffix"`
	// URLPrefix matches when the entry URL starts with it.
	URLPrefix string `yaml:"url_prefix"`
	// Author matches when it equals the entry author.
	Author string `yaml:"author"`
	// Meta matches when every pair equals the entry's metadata value
	// for that key (last duplicate wins). An empty map matches anything.
	Meta map[string]string `yaml:"meta"`
	// Action is required.
	Action string `yaml:"action"`
	// Guidance travels with the action into the dictation intent.
	Guidance string `yaml:"guidance"`
}

// Component holds one agent role's choices. An empty field falls back
// to the role's env file, then the process environment, then the
// built-in default.
type Component struct {
	// Agent is the agent binary.
	Agent string `yaml:"agent"`
	// Args is the argument template: whitespace-split, an element that
	// is exactly {prompt} or {tools} becomes that value as one argument.
	Args string `yaml:"args"`
	// Instructions is a file path (~ expands) whose content becomes the
	// prompt; it wins over the env-file prompt key.
	Instructions string `yaml:"instructions"`
	// AllowedTools is the agent's tool allowlist.
	AllowedTools string `yaml:"allowed_tools"`
	// Timeout is the run timeout in seconds; 0 means unset.
	Timeout int `yaml:"timeout"`
}

// Speakwrite is the speakwrite component plus the action map. A
// non-empty Actions list replaces the built-in map entirely; an empty
// or absent list keeps the built-ins.
type Speakwrite struct {
	Component `yaml:",inline"`
	Actions   []Action `yaml:"actions"`
}

// PublisherRule is one row of the actor's publisher routing list.
// Rules are consulted in order and the first match wins; when none
// matches, the built-in URL matching runs, skipping publishers a
// bare enabled: false rule disabled.
type PublisherRule struct {
	// Publisher names the backend: github-pr, slack-thread,
	// linear-issue, or exec.
	Publisher string `yaml:"publisher"`
	// URLPrefix matches when the record URL starts with it; empty
	// matches every URL.
	URLPrefix string `yaml:"url_prefix"`
	// Enabled defaults to true. false with no url_prefix disables the
	// publisher entirely, built-in matching included.
	Enabled *bool `yaml:"enabled"`
	// Command is the exec publisher's argv template: whitespace-split,
	// an element that is exactly {url} becomes the record URL as one
	// argument. Required for exec, forbidden otherwise.
	Command string `yaml:"command"`
}

// On reports the rule's enabled value, defaulting to true.
func (r PublisherRule) On() bool { return r.Enabled == nil || *r.Enabled }

// Thinkpol is the actor's key: routing rules only. Secrets stay in
// thinkpol.env.
type Thinkpol struct {
	Publishers []PublisherRule `yaml:"publishers"`
}

// Config is the telescreen.yaml schema, keyed by component. The
// thinkpol key is optional: the actor is deterministic and only its
// publisher routing is configurable.
type Config struct {
	Minitrue   Component  `yaml:"minitrue"`
	Speakwrite Speakwrite `yaml:"speakwrite"`
	Thinkpol   Thinkpol   `yaml:"thinkpol"`
}

// Load reads telescreen.yaml from the user config directory. A missing
// file falls back to the retired recdep/config.yaml; both missing
// returns the zero value; a malformed file returns an error.
func Load() (Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(dir, "telescreen.yaml")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return loadOld(filepath.Join(dir, "recdep", "config.yaml"))
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := decodeStrict(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Minitrue.Timeout < 0 {
		return Config{}, fmt.Errorf("%s: minitrue.timeout must not be negative: %d", path, c.Minitrue.Timeout)
	}
	if c.Speakwrite.Timeout < 0 {
		return Config{}, fmt.Errorf("%s: speakwrite.timeout must not be negative: %d", path, c.Speakwrite.Timeout)
	}
	if err := validateActions(c.Speakwrite.Actions); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := validatePublishers(c.Thinkpol.Publishers); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// loadOld reads the retired schema, one top-level actions list, and
// maps it to speakwrite.actions.
func loadOld(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var old struct {
		Actions []Action `yaml:"actions"`
	}
	if err := decodeStrict(b, &old); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := validateActions(old.Actions); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return Config{Speakwrite: Speakwrite{Actions: old.Actions}}, nil
}

func decodeStrict(b []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validateActions(actions []Action) error {
	for i, a := range actions {
		if a.Action == "" {
			return fmt.Errorf("actions[%d]: action is required", i)
		}
		for _, k := range slices.Sorted(maps.Keys(a.Meta)) {
			v := a.Meta[k]
			if !recdep.ValidMetaKey(k) {
				return fmt.Errorf("actions[%d]: meta key %q must be lowercase [a-z_]+", i, k)
			}
			if recdep.ReservedMetaKey(k) {
				return fmt.Errorf("actions[%d]: meta key %q is reserved by the queue contract", i, k)
			}
			if v == "" {
				return fmt.Errorf("actions[%d]: meta %s: a value is required", i, k)
			}
		}
	}
	return nil
}

// publisherNames is the closed set a routing rule may name; exec is
// the configured backend, the rest are built-ins.
var publisherNames = map[string]bool{
	"github-pr":    true,
	"slack-thread": true,
	"linear-issue": true,
	"exec":         true,
}

func validatePublishers(rules []PublisherRule) error {
	for i, r := range rules {
		if !publisherNames[r.Publisher] {
			return fmt.Errorf("thinkpol.publishers[%d]: unknown publisher %q", i, r.Publisher)
		}
		if r.Publisher == "exec" && len(strings.Fields(r.Command)) == 0 {
			return fmt.Errorf("thinkpol.publishers[%d]: exec requires a command", i)
		}
		if r.Publisher != "exec" && r.Command != "" {
			return fmt.Errorf("thinkpol.publishers[%d]: command is only for exec, not %s", i, r.Publisher)
		}
		// A disabled rule with a prefix would be inert: skipped in the
		// rule phase yet not disabling the built-in phase, so the
		// publisher the user believes off would still post.
		if r.Enabled != nil && !*r.Enabled && r.URLPrefix != "" {
			return fmt.Errorf("thinkpol.publishers[%d]: enabled: false cannot carry url_prefix; disable the publisher outright or drop the rule", i)
		}
	}
	return nil
}
