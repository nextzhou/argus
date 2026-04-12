package invariant

import (
	"fmt"

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
func InspectDirectory(dir string, workflowChecker func(id string) bool, allowReservedID func(id string) bool) (*InspectReport, error) {
	scanned, err := scanInvariantDirectory(dir, scanOptions{})
	if err != nil {
		return nil, err
	}

	report := &InspectReport{
		Valid: true,
		Files: make(map[string]*FileResult),
	}

	for _, entry := range scanned.entries {
		fr := &FileResult{Valid: true}
		if entry.inv != nil {
			fr.ID = entry.inv.ID
		}
		report.Files[entry.file] = fr

		for _, issue := range entry.issues {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    issue.Path,
				Message: issue.Message,
			})
		}

		if entry.inv == nil {
			if !fr.Valid {
				report.Valid = false
			}
			continue
		}

		if core.IsArgusReserved(entry.inv.ID) && !reservedIDAllowed(allowReservedID, entry.inv.ID) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    "id",
				Message: fmt.Sprintf("invariant ID %q uses reserved argus- prefix", entry.inv.ID),
			})
		}

		if entry.inv.Workflow != "" && !workflowChecker(entry.inv.Workflow) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    "workflow",
				Message: fmt.Sprintf("referenced workflow %q not found", entry.inv.Workflow),
			})
		}

		if !fr.Valid {
			report.Valid = false
		}
	}

	return report, nil
}

func reservedIDAllowed(allowReservedID func(id string) bool, id string) bool {
	return allowReservedID != nil && allowReservedID(id)
}
