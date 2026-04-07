package hook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/nextzhou/argus/internal/workspace"
	"gopkg.in/yaml.v3"
)

var tickSessionBaseDir = "/tmp/argus"

// HandleTick orchestrates the tick command logic.
// It reads stdin, determines project state, and writes context output.
// It always succeeds (errors become warning text) to maintain fail-open behavior.
func HandleTick(agent string, global bool, stdin io.Reader, stdout io.Writer, projectRoot string) error {
	_ = global

	input, err := ParseInput(stdin, agent)
	if err != nil {
		writeTickWarning(stdout, "could not parse hook input: %v", err)
		return nil
	}

	if IsSubAgent(input) {
		return nil
	}

	root, err := workspace.FindProjectRoot(projectRoot)
	if err != nil {
		writeTickWarning(stdout, "could not determine project root: %v", err)
		return nil
	}
	if root == nil {
		writeTickWarning(stdout, "not inside an Argus project")
		return nil
	}

	pipelinesDir := filepath.Join(root.Path, ".argus", "pipelines")
	activePipelines, scanWarnings, err := pipeline.ScanActivePipelines(pipelinesDir)
	if err != nil {
		writeTickWarning(stdout, "could not scan active pipelines: %v", err)
		return nil
	}
	if len(activePipelines) > 1 {
		writeTickWarning(stdout, "multiple active pipelines detected; run argus workflow cancel or argus doctor")
		return nil
	}

	sess, err := session.LoadSession(tickSessionBaseDir, input.SessionID)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			writeTickWarning(stdout, "could not load session state: %v", err)
			return nil
		}
		sess = &session.Session{}
	}

	firstTick := session.IsFirstTick(tickSessionBaseDir, input.SessionID)

	output, logDetails, snapshotPipelineID, snapshotJobID := buildTickOutput(root.Path, input.SessionID, sess, activePipelines, scanWarnings)

	failures := runTickInvariants(root.Path, firstTick)
	output = AppendInvariantFailed(output, failures)

	session.UpdateLastTick(sess, snapshotPipelineID, snapshotJobID, time.Now())
	if err := session.SaveSession(tickSessionBaseDir, input.SessionID, sess); err != nil {
		writeTickWarning(stdout, "could not save session state: %v", err)
		return nil
	}

	if err := LogHookExecution(root.Path, "tick", true, logDetails); err != nil {
		output = appendTickWarningText(output, fmt.Sprintf("could not write hook log: %v", err))
	}

	if _, err := io.WriteString(stdout, output); err != nil {
		return fmt.Errorf("writing tick output: %w", err)
	}

	return nil
}

func buildTickOutput(
	projectRoot string,
	sessionID string,
	sess *session.Session,
	activePipelines []pipeline.ActivePipeline,
	scanWarnings []pipeline.ScanWarning,
) (output string, logDetails string, snapshotPipelineID string, snapshotJobID string) {
	workflows := loadWorkflowSummaries(filepath.Join(projectRoot, ".argus", "workflows"))
	logDetails = fmt.Sprintf("active=%d warnings=%d", len(activePipelines), len(scanWarnings))

	if len(activePipelines) == 0 {
		return FormatNoPipeline(workflows), logDetails + " scenario=no-pipeline", "", ""
	}

	active := activePipelines[0]
	if session.IsSnoozed(sess, active.InstanceID) {
		return FormatSnoozed(workflows), logDetails + " scenario=snoozed", "", ""
	}

	if active.Pipeline.CurrentJob == nil {
		return appendTickWarningText(FormatNoPipeline(workflows), "active pipeline is missing current job state"), logDetails + " scenario=missing-current-job", active.InstanceID, ""
	}

	currentJobID := *active.Pipeline.CurrentJob
	snapshotPipelineID = active.InstanceID
	snapshotJobID = currentJobID

	workflowPath := filepath.Join(projectRoot, ".argus", "workflows", active.Pipeline.WorkflowID+".yaml")
	wf, err := loadWorkflowForTick(filepath.Join(projectRoot, ".argus", "workflows"), workflowPath)
	if err != nil {
		warning := fmt.Sprintf("could not load workflow %s: %v", active.Pipeline.WorkflowID, err)
		return appendTickWarningText(FormatNoPipeline(workflows), warning), logDetails + " scenario=workflow-load-error", snapshotPipelineID, snapshotJobID
	}

	jobIndex, found := pipeline.FindJobIndex(wf, currentJobID)
	if !found {
		warning := fmt.Sprintf("current job %s was not found in workflow %s", currentJobID, active.Pipeline.WorkflowID)
		return appendTickWarningText(FormatNoPipeline(workflows), warning), logDetails + " scenario=workflow-mismatch", snapshotPipelineID, snapshotJobID
	}

	progress := fmt.Sprintf("%d/%d", jobIndex+1, len(wf.Jobs))
	if session.HasStateChanged(sess, active.InstanceID, currentJobID) {
		prompt, skill := renderTickJobPrompt(active.Pipeline, wf, jobIndex)
		return FormatFullContext(active.InstanceID, active.Pipeline.WorkflowID, progress, currentJobID, prompt, skill, sessionID), logDetails + " scenario=full", snapshotPipelineID, snapshotJobID
	}

	return FormatMinimalSummary(active.Pipeline.WorkflowID, currentJobID, progress), logDetails + " scenario=minimal", snapshotPipelineID, snapshotJobID
}

func loadWorkflowSummaries(workflowsDir string) []WorkflowSummary {
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return nil
	}

	summaries := make([]WorkflowSummary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, "_") {
			continue
		}

		wf, err := workflow.ParseWorkflowFile(filepath.Join(workflowsDir, name))
		if err != nil {
			continue
		}

		summaries = append(summaries, WorkflowSummary{
			ID:          wf.ID,
			Description: wf.Description,
		})
	}

	return summaries
}

func renderTickJobPrompt(p *pipeline.Pipeline, wf *workflow.Workflow, jobIndex int) (prompt string, skill string) {
	if wf == nil || jobIndex < 0 || jobIndex >= len(wf.Jobs) {
		return "", ""
	}

	templateJobs := buildPipelineJobDataMap(p)
	tmplCtx := workflow.BuildContext(templateJobs, wf, jobIndex)
	renderedPrompt, _ := workflow.RenderPrompt(wf.Jobs[jobIndex].Prompt, tmplCtx)
	return renderedPrompt, wf.Jobs[jobIndex].Skill
}

func buildPipelineJobDataMap(p *pipeline.Pipeline) map[string]*workflow.PipelineJobData {
	if p == nil || p.Jobs == nil {
		return map[string]*workflow.PipelineJobData{}
	}

	templateJobs := make(map[string]*workflow.PipelineJobData, len(p.Jobs))
	for jobID, jobData := range p.Jobs {
		if jobData == nil {
			continue
		}
		templateJobs[jobID] = &workflow.PipelineJobData{
			StartedAt: jobData.StartedAt,
			EndedAt:   jobData.EndedAt,
			Message:   jobData.Message,
		}
	}

	return templateJobs
}

func runTickInvariants(projectRoot string, firstTick bool) []InvariantFailure {
	invariantsDir := filepath.Join(projectRoot, ".argus", "invariants")
	entries, err := os.ReadDir(invariantsDir)
	if err != nil {
		return nil
	}

	failures := make([]InvariantFailure, 0)
	ctx := context.Background()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, "_") {
			continue
		}

		inv, err := invariant.ParseInvariantFile(filepath.Join(invariantsDir, name))
		if err != nil || !shouldRunInvariantAuto(inv, firstTick) {
			continue
		}

		result := invariant.RunCheck(ctx, inv, projectRoot)
		if result.Passed {
			continue
		}

		failures = append(failures, InvariantFailure{
			ID:          inv.ID,
			Description: describeInvariant(inv),
			Suggestion:  invariantSuggestion(inv),
		})
	}

	return failures
}

func shouldRunInvariantAuto(inv *invariant.Invariant, firstTick bool) bool {
	if inv == nil {
		return false
	}

	if firstTick {
		return inv.Auto == "always" || inv.Auto == "session_start"
	}

	return inv.Auto == "always"
}

func describeInvariant(inv *invariant.Invariant) string {
	if inv == nil {
		return ""
	}
	if inv.Description != "" {
		return inv.Description
	}

	shells := make([]string, 0, len(inv.Check))
	for _, step := range inv.Check {
		shells = append(shells, step.Shell)
	}
	return strings.Join(shells, "; ")
}

func invariantSuggestion(inv *invariant.Invariant) string {
	if inv == nil {
		return "Review the invariant definition and project state"
	}
	if inv.Workflow != "" {
		return fmt.Sprintf("Run argus workflow start %s", inv.Workflow)
	}
	if inv.Prompt != "" {
		return inv.Prompt
	}
	return "Review the invariant definition and project state"
}

func loadWorkflowForTick(workflowsDir, workflowPath string) (*workflow.Workflow, error) {
	wf, err := workflow.ParseWorkflowFile(workflowPath)
	if err != nil {
		return nil, err
	}

	resolved, err := resolveTickWorkflowRefs(workflowsDir, workflowPath, wf)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

func resolveTickWorkflowRefs(workflowsDir, workflowPath string, wf *workflow.Workflow) (*workflow.Workflow, error) {
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

	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("re-reading workflow for ref resolution: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing workflow nodes: %w", err)
	}

	jobNodes := findTickJobNodes(&doc)
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

func findTickJobNodes(doc *yaml.Node) []*yaml.Node {
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

func writeTickWarning(stdout io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stdout, "[Argus] Warning: "+format+"\n", args...)
}

func appendTickWarningText(base string, warning string) string {
	if warning == "" {
		return base
	}
	if base == "" {
		return fmt.Sprintf("[Argus] Warning: %s\n", warning)
	}
	if strings.HasSuffix(base, "\n") {
		return base + fmt.Sprintf("[Argus] Warning: %s\n", warning)
	}
	return base + "\n" + fmt.Sprintf("[Argus] Warning: %s\n", warning)
}
