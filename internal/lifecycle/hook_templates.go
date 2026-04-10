// Package lifecycle provides setup, teardown, and asset release functionality for Argus.
package lifecycle

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/nextzhou/argus/internal/assets"
)

// HookTemplateData contains the variables used when rendering hook templates.
type HookTemplateData struct {
	Global bool
}

// RenderHookTemplate reads the hook template for the given agent from embedded assets
// and renders it with the provided data.
// Supported agents: "claude-code", "codex", "opencode".
func RenderHookTemplate(agent string, global bool) ([]byte, error) {
	templateFile, err := templatePathForAgent(agent)
	if err != nil {
		return nil, err
	}

	raw, err := assets.ReadAsset(templateFile)
	if err != nil {
		return nil, fmt.Errorf("reading hook template for %q: %w", agent, err)
	}

	tmpl, err := template.New(agent).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing hook template for %q: %w", agent, err)
	}

	data := HookTemplateData{Global: global}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering hook template for %q: %w", agent, err)
	}

	return buf.Bytes(), nil
}

func templatePathForAgent(agent string) (string, error) {
	switch agent {
	case "claude-code":
		return "hooks/claude-code.json.tmpl", nil
	case "codex":
		return "hooks/codex.json.tmpl", nil
	case "opencode":
		return "hooks/opencode.ts.tmpl", nil
	default:
		return "", fmt.Errorf("unsupported agent %q: must be claude-code, codex, or opencode", agent)
	}
}
