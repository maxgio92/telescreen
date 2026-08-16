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
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// Config is the telescreen.yaml schema, keyed by component. thinkpol
// has no key: the actor is deterministic and its secrets stay in
// thinkpol.env.
type Config struct {
	Minitrue   Component  `yaml:"minitrue"`
	Speakwrite Speakwrite `yaml:"speakwrite"`
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
	}
	return nil
}
