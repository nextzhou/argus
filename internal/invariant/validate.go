package invariant

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/nextzhou/argus/internal/core"
)

// InspectEntry is one source-aware invariant inspection entry.
type InspectEntry struct {
	Source   core.SourceRef `json:"source"`
	Valid    bool           `json:"valid"`
	Findings []core.Finding `json:"findings,omitempty"`
	ID       string         `json:"id,omitempty"`
}

// InspectReport is the result of inspecting an invariant directory.
type InspectReport struct {
	Valid   bool           `json:"valid"`
	Entries []InspectEntry `json:"entries"`
}

// InspectDirectory validates all *.yaml files in dir and returns an InspectReport.
// workflowChecker is called for each invariant that references a workflow to verify it exists.
func InspectDirectory(_ string, dir string, workflowChecker func(id string) bool, allowReservedID func(id string) bool) (*InspectReport, error) {
	scanned, err := scanInvariantDirectory(dir, scanOptions{})
	if err != nil {
		return nil, err
	}

	report := &InspectReport{
		Valid:   true,
		Entries: []InspectEntry{},
	}

	type reportEntry struct {
		InspectEntry
	}

	entriesByName := make(map[string]*reportEntry, len(scanned.entries))

	for _, entry := range scanned.entries {
		source := invariantSourceRef(dir, entry.file)
		reportEntry := &reportEntry{InspectEntry: InspectEntry{Source: source, Valid: true}}
		if entry.inv != nil {
			reportEntry.ID = entry.inv.ID
		}
		entriesByName[entry.file] = reportEntry

		for _, issue := range entry.issues {
			reportEntry.Valid = false
			appendInspectFinding(&reportEntry.Findings, source, issue.Kind, issue.Path, issue.Message)
		}

		if entry.inv == nil {
			if !reportEntry.Valid {
				report.Valid = false
			}
			continue
		}

		if core.IsArgusReserved(entry.inv.ID) && !reservedIDAllowed(allowReservedID, entry.inv.ID) {
			reportEntry.Valid = false
			appendInspectFinding(&reportEntry.Findings, source, "reserved_id", "id", fmt.Sprintf("invariant ID %q uses reserved argus- prefix", entry.inv.ID))
		}

		if entry.inv.Workflow != "" && !workflowChecker(entry.inv.Workflow) {
			reportEntry.Valid = false
			appendInspectFinding(&reportEntry.Findings, source, "missing_workflow", "workflow", fmt.Sprintf("referenced workflow %q not found", entry.inv.Workflow))
		}

		if !reportEntry.Valid {
			report.Valid = false
		}
	}

	fileNames := make([]string, 0, len(entriesByName))
	for name := range entriesByName {
		fileNames = append(fileNames, name)
	}
	slices.Sort(fileNames)
	for _, name := range fileNames {
		entry := entriesByName[name]
		entryCopy := entry.InspectEntry
		entryCopy.Findings = append([]core.Finding(nil), entry.Findings...)
		report.Entries = append(report.Entries, entryCopy)
	}

	return report, nil
}

func reservedIDAllowed(allowReservedID func(id string) bool, id string) bool {
	return allowReservedID != nil && allowReservedID(id)
}

func appendInspectFinding(findings *[]core.Finding, source core.SourceRef, code, fieldPath, message string) {
	*findings = append(*findings, core.Finding{
		Code:      code,
		Message:   message,
		Source:    source,
		FieldPath: fieldPath,
	})
}

func invariantSourceRef(dir, file string) core.SourceRef {
	absPath, err := filepath.Abs(filepath.Join(dir, file))
	if err != nil {
		absPath = filepath.Clean(filepath.Join(dir, file))
	}
	return core.SourceRef{
		Kind: core.SourceFile,
		Raw:  absPath,
	}
}
