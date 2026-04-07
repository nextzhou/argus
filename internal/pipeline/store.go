package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"gopkg.in/yaml.v3"
)

// ActivePipeline pairs an instance ID with its parsed pipeline data.
type ActivePipeline struct {
	InstanceID string
	Pipeline   *Pipeline
}

// ScanWarning records a file that could not be parsed during directory scan.
type ScanWarning struct {
	InstanceID string
	Err        error
}

// NewInstanceID builds a pipeline instance ID from a workflow ID and timestamp.
func NewInstanceID(workflowID string, t time.Time) string {
	return workflowID + "-" + core.FormatTimestamp(t)
}

// LoadPipeline reads and parses a pipeline data file from disk.
func LoadPipeline(dir, instanceID string) (*Pipeline, error) {
	path := filepath.Join(dir, instanceID+".yaml")

	if err := core.ValidatePath(dir, path); err != nil {
		return nil, fmt.Errorf("validating pipeline path: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening pipeline file %q: %w", instanceID, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var p Pipeline
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parsing pipeline YAML %q: %w", instanceID, err)
	}

	if err := core.CheckCompatibility(p.Version); err != nil {
		return nil, fmt.Errorf("pipeline %q version check: %w", instanceID, err)
	}

	return &p, nil
}

// SavePipeline writes a pipeline data file to disk, creating the directory if needed.
func SavePipeline(dir, instanceID string, p *Pipeline) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating pipeline directory: %w", err)
	}

	path := filepath.Join(dir, instanceID+".yaml")

	if err := core.ValidatePath(dir, path); err != nil {
		return fmt.Errorf("validating pipeline path: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling pipeline %q: %w", instanceID, err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing pipeline file %q: %w", instanceID, err)
	}

	return nil
}

// ScanActivePipelines reads all YAML files in dir and returns those with status "running".
// Corrupt files are skipped and reported as warnings. A nonexistent directory returns empty results.
func ScanActivePipelines(dir string) ([]ActivePipeline, []ScanWarning, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading pipeline directory: %w", err)
	}

	var actives []ActivePipeline
	var warnings []ScanWarning

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		instanceID := strings.TrimSuffix(entry.Name(), ".yaml")

		p, loadErr := LoadPipeline(dir, instanceID)
		if loadErr != nil {
			warnings = append(warnings, ScanWarning{
				InstanceID: instanceID,
				Err:        loadErr,
			})
			continue
		}

		if p.Status == "running" {
			actives = append(actives, ActivePipeline{
				InstanceID: instanceID,
				Pipeline:   p,
			})
		}
	}

	return actives, warnings, nil
}
