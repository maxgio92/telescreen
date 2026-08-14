// Package config loads the user configuration for the telescreen:
// structured tables that override built-in defaults. The file lives at
// <user config dir>/recdep/config.yaml; a missing file means defaults.
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
	// Action is required.
	Action string `yaml:"action"`
}

// Config is the config.yaml schema, version 1. A non-empty Actions list
// replaces the built-in action map entirely; an empty or absent list
// keeps the built-ins.
type Config struct {
	Actions []Action `yaml:"actions"`
}

// Load reads config.yaml from the user config directory. A missing file
// returns the zero value; a malformed file returns an error.
func Load() (Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(dir, "recdep", "config.yaml")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	for i, a := range c.Actions {
		if a.Action == "" {
			return Config{}, fmt.Errorf("%s: actions[%d]: action is required", path, i)
		}
	}
	return c, nil
}
