package core

import (
	"os"
	"path/filepath"
	"strings"
)

// SourceKind identifies the backing source for a diagnostic/report item.
type SourceKind string

const (
	// SourceFile identifies a regular filesystem-backed source.
	SourceFile SourceKind = "file"
	// SourceEmbeddedAsset identifies a built-in embedded asset source.
	SourceEmbeddedAsset SourceKind = "embedded_asset"
	// SourceSynthetic identifies a non-file synthetic or constructed source.
	SourceSynthetic SourceKind = "synthetic"
)

// SourceRef identifies the raw source backing a diagnostic/report item.
type SourceRef struct {
	Kind SourceKind `json:"kind"`
	Raw  string     `json:"raw"`
}

// Finding captures one structured diagnostic item.
type Finding struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Source    SourceRef `json:"source"`
	FieldPath string    `json:"field_path,omitempty"`
}

// FormatSourceRef renders source text for human-readable output.
// File sources render as only the formatted path.
// Non-file sources render with a simple kind prefix.
func FormatSourceRef(projectRoot string, source SourceRef) string {
	switch source.Kind {
	case SourceEmbeddedAsset:
		return "embedded asset: " + source.Raw
	case SourceSynthetic:
		return "synthetic: " + source.Raw
	case SourceFile:
		return formatFileSource(projectRoot, source.Raw)
	default:
		return source.Raw
	}
}

func formatFileSource(projectRoot, raw string) string {
	cleanRaw := filepath.Clean(raw)
	if projectRoot != "" {
		rel, err := filepath.Rel(projectRoot, cleanRaw)
		if err == nil && rel != ".." && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		cleanHome := filepath.Clean(homeDir)
		if cleanRaw == cleanHome {
			return "~"
		}
		prefix := cleanHome + string(filepath.Separator)
		if suffix, ok := strings.CutPrefix(cleanRaw, prefix); ok {
			return "~" + string(filepath.Separator) + suffix
		}
	}

	return cleanRaw
}
