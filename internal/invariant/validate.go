package invariant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/core"
)

// FieldError represents a single validation error in a file.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// FileResult represents inspection results for a single invariant file.
type FileResult struct {
	Valid  bool         `json:"valid"`
	Errors []FieldError `json:"errors,omitempty"`
	ID     string       `json:"id,omitempty"`
}

// InspectReport is the result of inspecting an invariant directory.
type InspectReport struct {
	Valid bool                   `json:"valid"`
	Files map[string]*FileResult `json:"files"`
}

// InspectDirectory validates all *.yaml files in dir and returns an InspectReport.
// workflowChecker is called for each invariant that references a workflow to verify it exists.
func InspectDirectory(dir string, workflowChecker func(id string) bool) (*InspectReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading invariant directory %q: %w", dir, err)
	}

	report := &InspectReport{
		Valid: true,
		Files: make(map[string]*FileResult),
	}

	// Phase 1: parse each YAML file, collect parsed invariants and per-file results.
	type parsedEntry struct {
		filename string
		inv      *Invariant
	}
	var parsed []parsedEntry

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		fr := &FileResult{Valid: true}
		report.Files[entry.Name()] = fr

		fullPath := filepath.Join(dir, entry.Name())
		inv, parseErr := ParseInvariantFile(fullPath)
		if parseErr != nil {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    entry.Name(),
				Message: parseErr.Error(),
			})
			report.Valid = false
			continue
		}

		fr.ID = inv.ID
		parsed = append(parsed, parsedEntry{filename: entry.Name(), inv: inv})
	}

	// Phase 2: cross-file duplicate ID detection.
	idToFiles := make(map[string][]string)
	for _, p := range parsed {
		idToFiles[p.inv.ID] = append(idToFiles[p.inv.ID], p.filename)
	}
	for id, files := range idToFiles {
		if len(files) > 1 {
			for _, fname := range files {
				fr := report.Files[fname]
				fr.Valid = false
				fr.Errors = append(fr.Errors, FieldError{
					Path:    fname,
					Message: fmt.Sprintf("duplicate invariant ID %q found in files: %s", id, strings.Join(files, ", ")),
				})
			}
			report.Valid = false
		}
	}

	// Phase 3: per-file cross-domain checks on successfully parsed invariants.
	for _, p := range parsed {
		fr := report.Files[p.filename]

		if core.IsArgusReserved(p.inv.ID) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    p.filename,
				Message: fmt.Sprintf("invariant ID %q uses reserved argus- prefix", p.inv.ID),
			})
			report.Valid = false
		}

		if p.inv.Workflow != "" && !workflowChecker(p.inv.Workflow) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    p.filename,
				Message: fmt.Sprintf("referenced workflow %q not found", p.inv.Workflow),
			})
			report.Valid = false
		}
	}

	return report, nil
}
