package hook

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/nextzhou/argus/internal/assets"
)

// renderTemplate reads an embedded template asset at templatePath, parses it,
// and executes it with the provided data.
// templatePath is relative to the assets root (e.g., "prompts/tick-minimal.md.tmpl").
func renderTemplate(templatePath string, data any) (string, error) {
	raw, err := assets.ReadAsset(templatePath)
	if err != nil {
		return "", fmt.Errorf("reading template %q: %w", templatePath, err)
	}

	tmpl, err := template.New(templatePath).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parsing template %q: %w", templatePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %q: %w", templatePath, err)
	}

	return buf.String(), nil
}
