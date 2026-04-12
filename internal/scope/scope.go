// Package scope provides filesystem-backed access to Argus artifact roots.
package scope

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/workflow"
	"gopkg.in/yaml.v3"
)

// Scope exposes artifact loading and write locations for a resolved root.
type Scope interface {
	LoadInvariantCatalog() (*invariant.Catalog, error)
	ScanActivePipelines() ([]pipeline.ActivePipeline, []pipeline.ScanWarning, error)
	LoadWorkflow(id string) (*workflow.Workflow, error)
	LoadWorkflowSummaries() ([]WorkflowSummary, error)

	ProjectRoot() string

	PipelinesDir() string
	WorkflowsDir() string
	LogsDir() string
}

// WorkflowSummary contains list output for available workflows in a scope.
type WorkflowSummary struct {
	ID          string
	Description string
	Jobs        int
}

type fsScope struct {
	root         string
	projectRoot  string
	pipelinesDir string
	workflowsDir string
	logsDir      string
}

var _ Scope = (*fsScope)(nil)

// NewProjectScope builds a scope rooted at <projectRoot>/.argus.
func NewProjectScope(projectRoot string) Scope {
	root := filepath.Join(projectRoot, ".argus")
	return &fsScope{
		root:         root,
		projectRoot:  projectRoot,
		pipelinesDir: filepath.Join(root, "pipelines"),
		workflowsDir: filepath.Join(root, "workflows"),
		logsDir:      filepath.Join(root, "logs"),
	}
}

// NewGlobalScope builds a scope rooted at the global Argus config directory.
func NewGlobalScope(globalRoot, projectRoot string) Scope {
	return &fsScope{
		root:         globalRoot,
		projectRoot:  projectRoot,
		pipelinesDir: filepath.Join(globalRoot, "pipelines", core.ProjectPathToSafeID(projectRoot)),
		workflowsDir: filepath.Join(globalRoot, "workflows"),
		logsDir:      filepath.Join(globalRoot, "logs"),
	}
}

func (s *fsScope) LoadInvariantCatalog() (*invariant.Catalog, error) {
	invariantsDir := filepath.Join(s.root, "invariants")
	catalog, err := invariant.LoadCatalog(invariantsDir, true)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return invariant.EmptyCatalog(), nil
		}
		return nil, fmt.Errorf("loading invariant catalog: %w", err)
	}
	return catalog, nil
}

func (s *fsScope) ScanActivePipelines() ([]pipeline.ActivePipeline, []pipeline.ScanWarning, error) {
	return pipeline.ScanActivePipelines(s.pipelinesDir)
}

func (s *fsScope) LoadWorkflow(id string) (*workflow.Workflow, error) {
	workflowPath := filepath.Join(s.workflowsDir, id+".yaml")
	if err := core.ValidatePath(s.workflowsDir, workflowPath); err != nil {
		return nil, fmt.Errorf("validating workflow path: %w", err)
	}

	wf, err := workflow.ParseWorkflowFile(workflowPath)
	if err != nil {
		return nil, err
	}

	resolved, err := resolveWorkflowRefs(s.workflowsDir, workflowPath, wf)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

func (s *fsScope) LoadWorkflowSummaries() ([]WorkflowSummary, error) {
	entries, err := os.ReadDir(s.workflowsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading workflows directory: %w", err)
	}

	summaries := make([]WorkflowSummary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, "_") {
			continue
		}

		wf, err := workflow.ParseWorkflowFile(filepath.Join(s.workflowsDir, name))
		if err != nil {
			slog.Warn("skipping unparseable workflow file", "file", name, "error", err)
			continue
		}
		if !core.DefinitionFileNameMatchesID(name, wf.ID) {
			slog.Warn(
				"skipping workflow file with mismatched file name",
				"file",
				name,
				"id",
				wf.ID,
				"expected",
				core.ExpectedYAMLFileName(wf.ID),
			)
			continue
		}

		summaries = append(summaries, WorkflowSummary{
			ID:          wf.ID,
			Description: wf.Description,
			Jobs:        len(wf.Jobs),
		})
	}

	return summaries, nil
}

func (s *fsScope) ProjectRoot() string {
	return s.projectRoot
}

func (s *fsScope) PipelinesDir() string {
	return s.pipelinesDir
}

func (s *fsScope) WorkflowsDir() string {
	return s.workflowsDir
}

func (s *fsScope) LogsDir() string {
	return s.logsDir
}

func resolveWorkflowRefs(workflowsDir, workflowPath string, wf *workflow.Workflow) (*workflow.Workflow, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	hasRefs := false
	for _, job := range wf.Jobs {
		if job.Ref != "" {
			hasRefs = true
			break
		}
	}
	if !hasRefs {
		return wf, nil
	}

	shared, err := workflow.LoadShared(filepath.Join(workflowsDir, "_shared.yaml"))
	if err != nil {
		return nil, fmt.Errorf("loading shared definitions: %w", err)
	}

	if err := core.ValidatePath(workflowsDir, workflowPath); err != nil {
		return nil, fmt.Errorf("validating workflow path: %w", err)
	}

	//nolint:gosec // workflowPath is constrained to workflowsDir via ValidatePath before re-reading for ref resolution.
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("re-reading workflow for ref resolution: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing workflow nodes: %w", err)
	}

	jobNodes := findWorkflowJobNodes(&doc)
	resolved := *wf
	resolved.Jobs = append([]workflow.Job(nil), wf.Jobs...)
	for index, job := range resolved.Jobs {
		if job.Ref == "" || index >= len(jobNodes) {
			continue
		}

		resolvedJob, err := workflow.ResolveRef(jobNodes[index], shared)
		if err != nil {
			return nil, fmt.Errorf("resolving ref for job[%d]: %w", index, err)
		}
		resolved.Jobs[index] = *resolvedJob
	}

	return &resolved, nil
}

func findWorkflowJobNodes(doc *yaml.Node) []*yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	for index := 0; index < len(root.Content)-1; index += 2 {
		if root.Content[index].Value != "jobs" {
			continue
		}

		jobsNode := root.Content[index+1]
		if jobsNode.Kind != yaml.SequenceNode {
			return nil
		}
		return jobsNode.Content
	}

	return nil
}
