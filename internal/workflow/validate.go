package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/nextzhou/argus/internal/core"
)

// Meta contains metadata about a valid workflow.
type Meta struct {
	ID   string `json:"id"`
	Jobs int    `json:"jobs"`
}

// SharedMeta contains metadata about _shared.yaml.
type SharedMeta struct {
	Jobs []string `json:"jobs"`
}

// InspectEntry is one source-aware workflow inspection entry.
type InspectEntry struct {
	Source   core.SourceRef `json:"source"`
	Valid    bool           `json:"valid"`
	Findings []core.Finding `json:"findings,omitempty"`
	Workflow *Meta          `json:"workflow,omitempty"`
	Shared   *SharedMeta    `json:"shared,omitempty"`
}

// InspectReport is the result of inspecting a workflow directory.
type InspectReport struct {
	Valid   bool           `json:"valid"`
	Entries []InspectEntry `json:"entries"`
}

const sharedFileName = "_shared.yaml"

// InspectDirectory validates all *.yaml files in dir and returns an InspectReport.
func InspectDirectory(_ string, dir string, allowReservedID func(id string) bool) (*InspectReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflow directory %q: %w", dir, err)
	}

	report := &InspectReport{
		Valid:   true,
		Entries: []InspectEntry{},
	}

	type reportEntry struct {
		InspectEntry
	}

	entriesByName := make(map[string]*reportEntry)

	var shared SharedJobs
	sharedPath := filepath.Join(dir, sharedFileName)
	sharedSource := fileSourceRef(sharedPath)
	if _, statErr := os.Stat(sharedPath); statErr == nil {
		loaded, loadErr := LoadShared(sharedPath)
		if loadErr != nil {
			entry := &reportEntry{InspectEntry: InspectEntry{Source: sharedSource, Valid: false}}
			appendFinding(&entry.Findings, sharedSource, "parse_error", "", loadErr.Error())
			entriesByName[sharedFileName] = entry
			report.Valid = false
		} else {
			shared = loaded
			keys := slices.Collect(func(yield func(string) bool) {
				for k := range shared {
					if !yield(k) {
						return
					}
				}
			})
			slices.Sort(keys)
			entriesByName[sharedFileName] = &reportEntry{
				InspectEntry: InspectEntry{
					Source: sharedSource,
					Valid:  true,
					Shared: &SharedMeta{Jobs: keys},
				},
			}
		}
	}

	type parsedEntry struct {
		filename string
		wf       *Workflow
	}
	var parsed []parsedEntry

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if entry.Name() == sharedFileName {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		source := fileSourceRef(fullPath)
		reportEntry := &reportEntry{InspectEntry: InspectEntry{Source: source, Valid: true}}
		entriesByName[entry.Name()] = reportEntry

		wf, parseErr := ParseWorkflowFile(fullPath)
		if parseErr != nil {
			reportEntry.Valid = false
			appendFinding(&reportEntry.Findings, source, "parse_error", "", parseErr.Error())
			report.Valid = false
			continue
		}

		reportEntry.Workflow = &Meta{ID: wf.ID, Jobs: len(wf.Jobs)}
		parsed = append(parsed, parsedEntry{filename: entry.Name(), wf: wf})
	}

	idToFiles := make(map[string][]string)
	for _, p := range parsed {
		idToFiles[p.wf.ID] = append(idToFiles[p.wf.ID], p.filename)
	}
	for id, files := range idToFiles {
		if len(files) <= 1 {
			continue
		}
		for _, fname := range files {
			entry := entriesByName[fname]
			entry.Valid = false
			appendFinding(&entry.Findings, entry.Source, "duplicate_id", "id", fmt.Sprintf("duplicate workflow ID %q found in files: %s", id, strings.Join(files, ", ")))
		}
		report.Valid = false
	}

	for _, p := range parsed {
		entry := entriesByName[p.filename]

		if !core.DefinitionFileNameMatchesID(p.filename, p.wf.ID) {
			entry.Valid = false
			appendFinding(&entry.Findings, entry.Source, "filename_mismatch", "id", core.DefinitionFileNameMismatchMessage("workflow", p.filename, p.wf.ID))
			report.Valid = false
		}

		if core.IsArgusReserved(p.wf.ID) && !reservedIDAllowed(allowReservedID, p.wf.ID) {
			entry.Valid = false
			appendFinding(&entry.Findings, entry.Source, "reserved_id", "id", fmt.Sprintf("workflow ID %q uses reserved argus- prefix", p.wf.ID))
			report.Valid = false
		}

		for i, job := range p.wf.Jobs {
			if job.Ref != "" && shared[job.Ref] == nil {
				entry.Valid = false
				appendFinding(&entry.Findings, entry.Source, "missing_ref", fmt.Sprintf("jobs[%d].ref", i), fmt.Sprintf("ref %q not found in %s", job.Ref, sharedFileName))
				report.Valid = false
			}

			if job.Prompt != "" {
				if _, tmplErr := template.New("").Parse(job.Prompt); tmplErr != nil {
					entry.Valid = false
					appendFinding(&entry.Findings, entry.Source, "invalid_template", fmt.Sprintf("jobs[%d].prompt", i), fmt.Sprintf("invalid template syntax: %s", tmplErr.Error()))
					report.Valid = false
				}
			}
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

func appendFinding(findings *[]core.Finding, source core.SourceRef, code, fieldPath, message string) {
	*findings = append(*findings, core.Finding{
		Code:      code,
		Message:   message,
		Source:    source,
		FieldPath: fieldPath,
	})
}

func fileSourceRef(path string) core.SourceRef {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = filepath.Clean(path)
	}
	return core.SourceRef{
		Kind: core.SourceFile,
		Raw:  absPath,
	}
}
