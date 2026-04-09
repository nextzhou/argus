package hook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/session"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/nextzhou/argus/internal/workspace"
)

// HandleTick orchestrates the tick command logic.
// It reads stdin, determines project state, and writes context output.
// It always succeeds (errors become warning text) to maintain fail-open behavior.
func HandleTick(agent string, global bool, stdin io.Reader, stdout io.Writer, projectRoot string, sessionBaseDir string) error {
	input, err := ParseInput(stdin, agent)
	if err != nil {
		writeTickWarning(stdout, "could not parse hook input: %v", err)
		return nil
	}

	if IsSubAgent(input) {
		return nil
	}

	effectiveCWD := projectRoot
	if input.CWD != "" {
		effectiveCWD = input.CWD
	}

	root, err := workspace.FindProjectRoot(effectiveCWD)
	if err != nil {
		writeTickWarning(stdout, "could not determine project root: %v", err)
		return nil
	}
	if root == nil {
		if !global {
			writeTickWarning(stdout, "not inside an Argus project")
		}
		return nil
	}

	s, err := scope.ResolveScopeForTick(root, global)
	if err != nil {
		writeTickWarning(stdout, "scope resolution error: %v", err)
		_ = LogHookExecution("", "tick", false, "scope: "+err.Error())
		return nil
	}
	if s == nil {
		if !global {
			writeTickWarning(stdout, "not inside an Argus project")
		} else {
			_ = LogHookExecution("", "tick", true, "no-scope")
		}
		return nil
	}

	activePipelines, scanWarnings, err := s.ScanActivePipelines()
	if err != nil {
		writeTickWarning(stdout, "could not scan active pipelines: %v", err)
		return nil
	}
	if len(activePipelines) > 1 {
		writeTickWarning(stdout, "multiple active pipelines detected; run argus workflow cancel or argus doctor")
		return nil
	}

	sess, err := session.LoadSession(sessionBaseDir, input.SessionID)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			writeTickWarning(stdout, "could not load session state: %v", err)
			return nil
		}
		sess = &session.Session{}
	}

	firstTick := session.IsFirstTick(sessionBaseDir, input.SessionID)

	var (
		output             string
		logDetails         string
		snapshotPipelineID string
		snapshotJobID      string
	)
	if len(activePipelines) == 0 {
		output, logDetails = buildNoActivePipelineOutput(s, firstTick)
	} else {
		output, logDetails, snapshotPipelineID, snapshotJobID = buildActivePipelineOutput(
			s,
			input.SessionID,
			sess,
			activePipelines,
			scanWarnings,
		)
	}

	session.UpdateLastTick(sess, snapshotPipelineID, snapshotJobID, time.Now())
	if err := session.SaveSession(sessionBaseDir, input.SessionID, sess); err != nil {
		writeTickWarning(stdout, "could not save session state: %v", err)
		return nil
	}

	if err := LogHookExecution(s.LogsDir(), "tick", true, logDetails); err != nil {
		output = appendTickWarningText(output, fmt.Sprintf("could not write hook log: %v", err))
	}

	if _, err := io.WriteString(stdout, output); err != nil {
		return fmt.Errorf("writing tick output: %w", err)
	}

	return nil
}

func buildActivePipelineOutput(
	s scope.Scope,
	sessionID string,
	sess *session.Session,
	activePipelines []pipeline.ActivePipeline,
	scanWarnings []pipeline.ScanWarning,
) (output string, logDetails string, snapshotPipelineID string, snapshotJobID string) {
	logDetails = fmt.Sprintf("active=%d warnings=%d", len(activePipelines), len(scanWarnings))
	formatErrorOutput := func(err error, pipelineID string, jobID string) (string, string, string, string) {
		return appendTickWarningText("", fmt.Sprintf("format error: %v", err)), logDetails + " scenario=format-error", pipelineID, jobID
	}

	active := activePipelines[0]
	if session.IsSnoozed(sess, active.InstanceID) {
		workflows := loadTickWorkflowSummaries(s)
		if len(workflows) == 0 {
			return "", logDetails + " scenario=snoozed-no-output", "", ""
		}
		output, err := FormatSnoozed(workflows)
		if err != nil {
			return formatErrorOutput(err, "", "")
		}
		return output, logDetails + " scenario=snoozed", "", ""
	}

	if active.Pipeline.CurrentJob == nil {
		workflows := loadTickWorkflowSummaries(s)
		output, err := FormatNoPipeline(workflows)
		if err != nil {
			return formatErrorOutput(err, active.InstanceID, "")
		}
		return appendTickWarningText(output, "active pipeline is missing current job state"), logDetails + " scenario=missing-current-job", active.InstanceID, ""
	}

	currentJobID := *active.Pipeline.CurrentJob
	snapshotPipelineID = active.InstanceID
	snapshotJobID = currentJobID

	wf, err := s.LoadWorkflow(active.Pipeline.WorkflowID)
	if err != nil {
		warning := fmt.Sprintf("could not load workflow %s: %v", active.Pipeline.WorkflowID, err)
		workflows := loadTickWorkflowSummaries(s)
		output, formatErr := FormatNoPipeline(workflows)
		if formatErr != nil {
			return formatErrorOutput(formatErr, snapshotPipelineID, snapshotJobID)
		}
		return appendTickWarningText(output, warning), logDetails + " scenario=workflow-load-error", snapshotPipelineID, snapshotJobID
	}

	jobIndex, found := pipeline.FindJobIndex(wf, currentJobID)
	if !found {
		warning := fmt.Sprintf("current job %s was not found in workflow %s", currentJobID, active.Pipeline.WorkflowID)
		workflows := loadTickWorkflowSummaries(s)
		output, err := FormatNoPipeline(workflows)
		if err != nil {
			return formatErrorOutput(err, snapshotPipelineID, snapshotJobID)
		}
		return appendTickWarningText(output, warning), logDetails + " scenario=workflow-mismatch", snapshotPipelineID, snapshotJobID
	}

	progress := fmt.Sprintf("%d/%d", jobIndex+1, len(wf.Jobs))
	if session.HasStateChanged(sess, active.InstanceID, currentJobID) {
		prompt, skill := renderTickJobPrompt(active.Pipeline, wf, jobIndex)
		output, err := FormatFullContext(active.InstanceID, active.Pipeline.WorkflowID, progress, currentJobID, prompt, skill, sessionID)
		if err != nil {
			return formatErrorOutput(err, snapshotPipelineID, snapshotJobID)
		}
		return output, logDetails + " scenario=full", snapshotPipelineID, snapshotJobID
	}

	output, err = FormatMinimalSummary(active.Pipeline.WorkflowID, currentJobID, progress)
	if err != nil {
		return formatErrorOutput(err, snapshotPipelineID, snapshotJobID)
	}
	return output, logDetails + " scenario=minimal", snapshotPipelineID, snapshotJobID
}

func buildNoActivePipelineOutput(s scope.Scope, firstTick bool) (output string, logDetails string) {
	logDetails = "active=0 warnings=0"
	failure := runTickInvariants(s, firstTick)
	if failure != nil {
		output, err := FormatInvariantFailure(*failure)
		if err != nil {
			return appendTickWarningText("", fmt.Sprintf("format error: %v", err)), logDetails + " scenario=format-error"
		}
		return output, logDetails + " scenario=invariant-failed invariant=" + failure.ID
	}

	workflows := loadTickWorkflowSummaries(s)
	if len(workflows) == 0 {
		return "", logDetails + " scenario=no-output"
	}

	output, err := FormatNoPipeline(workflows)
	if err != nil {
		return appendTickWarningText("", fmt.Sprintf("format error: %v", err)), logDetails + " scenario=format-error"
	}
	return output, logDetails + " scenario=no-pipeline"
}

func loadTickWorkflowSummaries(s scope.Scope) []WorkflowSummary {
	if s == nil {
		return nil
	}

	summaries, err := s.LoadWorkflowSummaries()
	if err != nil {
		slog.Debug("tick: could not load workflow summaries", "error", err)
		return nil
	}
	return toHookWorkflowSummaries(summaries)
}

func toHookWorkflowSummaries(scopeSummaries []scope.WorkflowSummary) []WorkflowSummary {
	if len(scopeSummaries) == 0 {
		return nil
	}

	summaries := make([]WorkflowSummary, len(scopeSummaries))
	for index, summary := range scopeSummaries {
		summaries[index] = WorkflowSummary{
			ID:          summary.ID,
			Description: summary.Description,
		}
	}

	return summaries
}

func renderTickJobPrompt(p *pipeline.Pipeline, wf *workflow.Workflow, jobIndex int) (prompt string, skill string) {
	if wf == nil || jobIndex < 0 || jobIndex >= len(wf.Jobs) {
		return "", ""
	}

	templateJobs := buildPipelineJobDataMap(p)
	tmplCtx := workflow.BuildContext(templateJobs, wf, jobIndex)
	// RenderPrompt returns (rendered, warnings) where warnings is []string of
	// unresolved template placeholders — not an error. In tick's fail-open context,
	// partial template rendering is acceptable, so warnings are intentionally discarded.
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

func runTickInvariants(s scope.Scope, firstTick bool) *InvariantFailure {
	if s == nil {
		return nil
	}

	invariants, err := s.LoadInvariants()
	if err != nil {
		slog.Debug("tick: could not load invariants", "error", err)
		return nil
	}
	if len(invariants) == 0 {
		return nil
	}

	ctx := context.Background()
	for _, inv := range invariants {
		if !shouldRunInvariantAuto(inv, firstTick) {
			continue
		}

		result := invariant.RunCheck(ctx, inv, s.ProjectRoot())
		if result.Passed {
			continue
		}

		failure := InvariantFailure{
			ID:          inv.ID,
			Description: describeInvariant(inv),
			Suggestion:  invariantSuggestion(inv),
		}
		return &failure
	}

	return nil
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

func writeTickWarning(stdout io.Writer, format string, args ...any) {
	// Tick warnings share the same plain-text constraint as normal tick output:
	// do not begin with '[' or '{', or Codex may try to parse them as JSON.
	_, _ = fmt.Fprintf(stdout, "Argus warning: "+format+"\n", args...)
}

func appendTickWarningText(base string, warning string) string {
	if warning == "" {
		return base
	}
	if base == "" {
		return fmt.Sprintf("Argus warning: %s\n", warning)
	}
	if strings.HasSuffix(base, "\n") {
		return base + fmt.Sprintf("Argus warning: %s\n", warning)
	}
	return base + "\n" + fmt.Sprintf("Argus warning: %s\n", warning)
}
