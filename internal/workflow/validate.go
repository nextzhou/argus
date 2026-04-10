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

// FieldError represents a single validation error in a workflow file.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Meta contains metadata about a valid workflow.
type Meta struct {
	ID   string `json:"id"`
	Jobs int    `json:"jobs"`
}

// SharedMeta contains metadata about _shared.yaml.
type SharedMeta struct {
	Jobs []string `json:"jobs"`
}

// FileResult represents inspection results for a single workflow file or _shared.yaml.
type FileResult struct {
	Valid    bool         `json:"valid"`
	Errors   []FieldError `json:"errors,omitempty"`
	Workflow *Meta        `json:"workflow,omitempty"`
	Shared   *SharedMeta  `json:"shared,omitempty"`
}

// InspectReport is the result of inspecting a workflow directory.
type InspectReport struct {
	Valid bool                   `json:"valid"`
	Files map[string]*FileResult `json:"files"`
}

const sharedFileName = "_shared.yaml"

// InspectDirectory validates all *.yaml files in dir and returns an InspectReport.
func InspectDirectory(dir string, allowReservedID func(id string) bool) (*InspectReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflow directory %q: %w", dir, err)
	}

	report := &InspectReport{
		Valid: true,
		Files: make(map[string]*FileResult),
	}

	var shared SharedJobs
	sharedPath := filepath.Join(dir, sharedFileName)
	if _, statErr := os.Stat(sharedPath); statErr == nil {
		loaded, loadErr := LoadShared(sharedPath)
		if loadErr != nil {
			fr := &FileResult{Valid: false}
			fr.Errors = append(fr.Errors, FieldError{
				Path:    sharedFileName,
				Message: loadErr.Error(),
			})
			report.Files[sharedFileName] = fr
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
			report.Files[sharedFileName] = &FileResult{
				Valid:  true,
				Shared: &SharedMeta{Jobs: keys},
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

		fr := &FileResult{Valid: true}
		report.Files[entry.Name()] = fr

		fullPath := filepath.Join(dir, entry.Name())
		wf, parseErr := ParseWorkflowFile(fullPath)
		if parseErr != nil {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    entry.Name(),
				Message: parseErr.Error(),
			})
			report.Valid = false
			continue
		}

		fr.Workflow = &Meta{ID: wf.ID, Jobs: len(wf.Jobs)}
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
			fr := report.Files[fname]
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    "id",
				Message: fmt.Sprintf("duplicate workflow ID %q found in files: %s", id, strings.Join(files, ", ")),
			})
		}
		report.Valid = false
	}

	for _, p := range parsed {
		fr := report.Files[p.filename]

		if !core.DefinitionFileNameMatchesID(p.filename, p.wf.ID) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    "id",
				Message: core.DefinitionFileNameMismatchMessage("workflow", p.filename, p.wf.ID),
			})
			report.Valid = false
		}

		if core.IsArgusReserved(p.wf.ID) && !reservedIDAllowed(allowReservedID, p.wf.ID) {
			fr.Valid = false
			fr.Errors = append(fr.Errors, FieldError{
				Path:    "id",
				Message: fmt.Sprintf("workflow ID %q uses reserved argus- prefix", p.wf.ID),
			})
			report.Valid = false
		}

		for i, job := range p.wf.Jobs {
			if job.Ref != "" && shared[job.Ref] == nil {
				fr.Valid = false
				fr.Errors = append(fr.Errors, FieldError{
					Path:    fmt.Sprintf("jobs[%d].ref", i),
					Message: fmt.Sprintf("ref %q not found in %s", job.Ref, sharedFileName),
				})
				report.Valid = false
			}

			if job.Prompt != "" {
				if _, tmplErr := template.New("").Parse(job.Prompt); tmplErr != nil {
					fr.Valid = false
					fr.Errors = append(fr.Errors, FieldError{
						Path:    fmt.Sprintf("jobs[%d].prompt", i),
						Message: fmt.Sprintf("invalid template syntax: %s", tmplErr.Error()),
					})
					report.Valid = false
				}
			}
		}
	}

	return report, nil
}

func reservedIDAllowed(allowReservedID func(id string) bool, id string) bool {
	return allowReservedID != nil && allowReservedID(id)
}
