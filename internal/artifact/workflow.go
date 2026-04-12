package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/workflow"
	"gopkg.in/yaml.v3"
)

// WorkflowProvider provides workflow artifacts for one namespace.
type WorkflowProvider struct {
	projectRoot string
	dir         string
}

// NewWorkflowProvider creates a workflow provider for one artifact namespace.
func NewWorkflowProvider(projectRoot, dir string) *WorkflowProvider {
	return &WorkflowProvider{
		projectRoot: projectRoot,
		dir:         dir,
	}
}

// ProjectRoot returns the project root used for relative rendering and policy.
func (p *WorkflowProvider) ProjectRoot() string {
	if p == nil {
		return ""
	}
	return p.projectRoot
}

// Dir returns the backing workflow directory.
func (p *WorkflowProvider) Dir() string {
	if p == nil {
		return ""
	}
	return p.dir
}

// Load reads one workflow by ID and resolves any shared job refs.
func (p *WorkflowProvider) Load(id string) (*workflow.Workflow, error) {
	if p == nil {
		return nil, fmt.Errorf("workflow provider is nil")
	}

	workflowPath := filepath.Join(p.dir, id+".yaml")
	if err := core.ValidatePath(p.dir, workflowPath); err != nil {
		return nil, fmt.Errorf("validating workflow path: %w", err)
	}

	wf, err := workflow.ParseWorkflowFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("parsing workflow %q: %w", workflowPath, err)
	}

	resolved, err := resolveWorkflowRefs(p.dir, workflowPath, wf)
	if err != nil {
		return nil, fmt.Errorf("resolving workflow refs for %q: %w", workflowPath, err)
	}

	return resolved, nil
}

// Summaries lists parseable workflows that are addressable by their expected file names.
func (p *WorkflowProvider) Summaries() ([]WorkflowSummary, error) {
	if p == nil {
		return nil, fmt.Errorf("workflow provider is nil")
	}

	entries, err := os.ReadDir(p.dir)
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

		wf, err := workflow.ParseWorkflowFile(filepath.Join(p.dir, name))
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

// Exists reports whether a workflow exists at its expected path and parses cleanly.
func (p *WorkflowProvider) Exists(id string) bool {
	if p == nil {
		return false
	}

	if err := core.ValidateWorkflowID(id); err != nil {
		return false
	}

	path := filepath.Join(p.dir, core.ExpectedYAMLFileName(id))
	if err := core.ValidatePath(p.dir, path); err != nil {
		return false
	}

	wf, err := workflow.ParseWorkflowFile(path)
	if err != nil {
		return false
	}

	return wf.ID == id
}

// Inspect validates all workflows visible through this provider.
func (p *WorkflowProvider) Inspect(allowReservedID func(id string) bool) (*workflow.InspectReport, error) {
	if p == nil {
		return nil, fmt.Errorf("workflow provider is nil")
	}
	report, err := workflow.InspectDirectory(p.projectRoot, p.dir, allowReservedID)
	if err != nil {
		return nil, fmt.Errorf("inspecting workflows in %s: %w", p.dir, err)
	}
	return report, nil
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

	//nolint:gosec // workflowPath is constrained to workflowsDir via ValidatePath above.
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
