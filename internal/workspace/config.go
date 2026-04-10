// Package workspace provides workspace configuration and path utilities.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds workspace-level settings persisted as YAML.
type Config struct {
	Workspaces []string `yaml:"workspaces"`
}

// DeduplicateWorkspaces removes duplicate workspace paths from the list.
// Two paths are considered duplicates if they are equal as stored (normalized form).
// The first occurrence is kept; subsequent duplicates are removed.
// The relative order of unique paths is preserved.
func DeduplicateWorkspaces(workspaces []string) []string {
	seen := make(map[string]bool, len(workspaces))
	result := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		if !seen[ws] {
			seen[ws] = true
			result = append(result, ws)
		}
	}
	return result
}

// LoadConfig reads a workspace config file, rejecting unknown fields.
func LoadConfig(path string) (*Config, error) {
	//nolint:gosec // LoadConfig intentionally reads the exact config path selected by the caller.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing config YAML %q: %w", path, err)
	}

	return &c, nil
}

// SaveConfig writes a workspace config file, creating parent directories as needed.
func SaveConfig(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}

	return nil
}
