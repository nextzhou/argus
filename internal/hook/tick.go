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

	"github.com/nextzhou/argus/internal/artifact"
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
func HandleTick(ctx context.Context, agent string, global bool, stdin io.Reader, stdout io.Writer, projectRoot string, sessionBaseDir string) error {
	return HandleTickWithSessionStore(ctx, agent, global, stdin, stdout, projectRoot, session.NewFileStore(sessionBaseDir))
}

// HandleTickWithSessionStore orchestrates the tick command logic using store.
func HandleTickWithSessionStore(ctx context.Context, agent string, global bool, stdin io.Reader, stdout io.Writer, projectRoot string, store session.Store) error {
	input, err := ParseInput(stdin, agent)
	if err != nil {
		writeTickWarningf(stdout, "could not parse hook input: %v", err)
		return nil
	}

	return HandleTickInputWithSessionStore(ctx, global, input, stdout, projectRoot, store)
}

// HandleTickInputWithSessionStore orchestrates the tick command logic from a
// caller-provided input instead of parsing stdin. It is used by debug paths such
// as `argus tick --mock` so they can reuse the normal tick behavior.
func HandleTickInputWithSessionStore(ctx context.Context, global bool, input *AgentInput, stdout io.Writer, projectRoot string, store session.Store) error {
	if input == nil {
		return fmt.Errorf("tick input is nil")
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
		writeTickWarningf(stdout, "could not determine project root: %v", err)
		return nil
	}
	if root == nil {
		if !global {
			writeTickWarningf(stdout, "not inside an Argus project")
		}
		return nil
	}

	s, err := scope.ResolveScopeForTick(root, global)
	if err != nil {
		writeTickWarningf(stdout, "scope resolution error: %v", err)
		logStore, logErr := artifact.NewFallbackHookLogStore()
		if logErr == nil {
			_ = logStore.Append("tick", false, "scope: "+err.Error())
		}
		return nil
	}
	if s == nil {
		if !global {
			writeTickWarningf(stdout, "not inside an Argus project")
		} else {
			logStore, logErr := artifact.NewFallbackHookLogStore()
			if logErr == nil {
				_ = logStore.Append("tick", true, "no-scope")
			}
		}
		return nil
	}

	activePipelines, scanWarnings, err := s.Artifacts().Pipelines().ScanActive()
	if err != nil {
		writeTickWarningf(stdout, "could not scan active pipelines: %v", err)
		return nil
	}
	if len(activePipelines) > 1 {
		output, formatErr := FormatMultipleActivePipelines(activePipelineIDs(activePipelines), input.SessionID)
		if formatErr != nil {
			writeTickWarningf(stdout, "multiple active pipelines detected; run argus workflow cancel or argus doctor")
			return nil
		}
		if _, err := io.WriteString(stdout, output); err != nil {
			return fmt.Errorf("writing tick output: %w", err)
		}
		return nil
	}

	sess, err := store.Load(input.SessionID)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			writeTickWarningf(stdout, "could not load session state: %v", err)
			return nil
		}
		sess = &session.Session{}
	}

	firstTick := session.IsFirstTickWithStore(store, input.SessionID)

	var (
		output             string
		logDetails         string
		snapshotPipelineID string
		snapshotJobID      string
	)
	if len(activePipelines) == 0 {
		output, logDetails = buildNoActivePipelineOutput(ctx, s, sess, firstTick)
	} else {
		output, logDetails, snapshotPipelineID, snapshotJobID = buildActivePipelineOutput(
			ctx,
			s,
			input.SessionID,
			sess,
			activePipelines,
			scanWarnings,
		)
	}

	session.UpdateLastTick(sess, snapshotPipelineID, snapshotJobID, time.Now())
	if err := store.Save(input.SessionID, sess); err != nil {
		writeTickWarningf(stdout, "could not save session state: %v", err)
		return nil
	}

	if err := s.Artifacts().HookLog().Append("tick", true, logDetails); err != nil {
		output = appendTickWarningText(output, fmt.Sprintf("could not write hook log: %v", err))
	}

	if _, err := io.WriteString(stdout, output); err != nil {
		return fmt.Errorf("writing tick output: %w", err)
	}

	return nil
}

func buildActivePipelineOutput(
	ctx context.Context,
	s *scope.Resolved,
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
		output, err := FormatSnoozed(workflows)
		if err != nil {
			return formatErrorOutput(err, "", "")
		}
		return output, logDetails + " scenario=snoozed", "", ""
	}

	if active.Pipeline.CurrentJob == nil {
		output, err := FormatActivePipelineIssue(ActivePipelineIssue{
			PipelineID:          active.InstanceID,
			WorkflowID:          active.Pipeline.WorkflowID,
			Issue:               "The active pipeline is missing current job state.",
			InvestigateCommand:  "argus status",
			InvestigateGuidance: "inspect the current pipeline state",
			SessionID:           sessionID,
		})
		if err != nil {
			return formatErrorOutput(err, active.InstanceID, "")
		}
		return output, logDetails + " scenario=missing-current-job", active.InstanceID, ""
	}

	currentJobID := *active.Pipeline.CurrentJob
	snapshotPipelineID = active.InstanceID
	snapshotJobID = currentJobID

	wf, err := s.Artifacts().Workflows().Load(active.Pipeline.WorkflowID)
	if err != nil {
		output, formatErr := FormatActivePipelineIssue(ActivePipelineIssue{
			PipelineID:          snapshotPipelineID,
			WorkflowID:          active.Pipeline.WorkflowID,
			Issue:               fmt.Sprintf("could not load workflow %s: %v", active.Pipeline.WorkflowID, err),
			InvestigateCommand:  "argus doctor",
			InvestigateGuidance: "diagnose the broken workflow reference or local Argus state",
			SessionID:           sessionID,
		})
		if formatErr != nil {
			return formatErrorOutput(formatErr, snapshotPipelineID, snapshotJobID)
		}
		return output, logDetails + " scenario=workflow-load-error", snapshotPipelineID, snapshotJobID
	}

	jobIndex, found := pipeline.FindJobIndex(wf, currentJobID)
	if !found {
		output, err := FormatActivePipelineIssue(ActivePipelineIssue{
			PipelineID:          snapshotPipelineID,
			WorkflowID:          active.Pipeline.WorkflowID,
			Issue:               fmt.Sprintf("current job %s was not found in workflow %s", currentJobID, active.Pipeline.WorkflowID),
			InvestigateCommand:  "argus status",
			InvestigateGuidance: "inspect the current pipeline state before deciding whether to cancel it",
			SessionID:           sessionID,
		})
		if err != nil {
			return formatErrorOutput(err, snapshotPipelineID, snapshotJobID)
		}
		return output, logDetails + " scenario=workflow-mismatch", snapshotPipelineID, snapshotJobID
	}

	progress := fmt.Sprintf("%d/%d", jobIndex+1, len(wf.Jobs))
	if session.HasStateChanged(sess, active.InstanceID, currentJobID) {
		prompt, skill := renderTickJobPrompt(ctx, active.Pipeline, wf, jobIndex)
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

type tickInvariantRun struct {
	Failure   *InvariantFailure
	TotalTime time.Duration
	RanChecks int
}

func buildNoActivePipelineOutput(ctx context.Context, s *scope.Resolved, sess *session.Session, firstTick bool) (output string, logDetails string) {
	logDetails = "active=0 warnings=0"
	catalog, warning := loadTickInvariantCatalog(s)
	if catalog != nil && len(catalog.Issues) > 0 {
		logDetails += fmt.Sprintf(" invalid_invariants=%d", len(catalog.Issues))
	}

	result := runTickInvariants(ctx, catalog, s.ProjectRoot(), firstTick)
	if result.Failure != nil {
		output, err := FormatInvariantFailure(*result.Failure)
		if err != nil {
			return appendTickWarningText("", fmt.Sprintf("format error: %v", err)), logDetails + " scenario=format-error"
		}
		if warning != "" {
			output = appendTickWarningText(output, warning)
		}
		invariantID := ""
		if result.Failure.Invariant != nil {
			invariantID = result.Failure.Invariant.ID
		}
		return output, logDetails + " scenario=invariant-failed invariant=" + invariantID
	}

	slowWarning := ""
	if result.RanChecks > 0 && result.TotalTime > invariant.SlowCheckThreshold && !session.HasWarnedSlowCheck(sess, s.ProjectRoot()) {
		slowWarning = formatSlowInvariantWarning(result.TotalTime)
		session.MarkSlowCheckWarned(sess, s.ProjectRoot())
		logDetails += fmt.Sprintf(" slow_check=%.1fs", result.TotalTime.Seconds())
	}

	workflows := loadTickWorkflowSummaries(s)
	if len(workflows) == 0 {
		output = appendTickWarningText(output, warning)
		output = appendTickWarningText(output, slowWarning)
		return output, logDetails + " scenario=no-output"
	}

	output, err := FormatNoPipeline(workflows)
	if err != nil {
		return appendTickWarningText("", fmt.Sprintf("format error: %v", err)), logDetails + " scenario=format-error"
	}
	if warning != "" {
		output = appendTickWarningText(output, warning)
	}
	if slowWarning != "" {
		output = appendTickWarningText(output, slowWarning)
	}
	return output, logDetails + " scenario=no-pipeline"
}

func loadTickWorkflowSummaries(s *scope.Resolved) []WorkflowSummary {
	if s == nil {
		return nil
	}

	summaries, err := s.Artifacts().Workflows().Summaries()
	if err != nil {
		slog.Debug("tick: could not load workflow summaries", "error", err)
		return nil
	}
	return toHookWorkflowSummaries(summaries)
}

func toHookWorkflowSummaries(scopeSummaries []artifact.WorkflowSummary) []WorkflowSummary {
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

func renderTickJobPrompt(ctx context.Context, p *pipeline.Pipeline, wf *workflow.Workflow, jobIndex int) (prompt string, skill string) {
	if wf == nil || jobIndex < 0 || jobIndex >= len(wf.Jobs) {
		return "", ""
	}

	templateJobs := buildPipelineJobDataMap(p)
	tmplCtx := workflow.BuildContext(ctx, templateJobs, wf, jobIndex)
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

func loadTickInvariantCatalog(s *scope.Resolved) (*invariant.Catalog, string) {
	if s == nil {
		return invariant.EmptyCatalog(), ""
	}

	catalog, err := s.Artifacts().Invariants().Catalog(true)
	if err != nil {
		slog.Debug("tick: could not load invariants", "error", err)
		return invariant.EmptyCatalog(), fmt.Sprintf("could not load invariants: %v", err)
	}
	if catalog == nil {
		return invariant.EmptyCatalog(), ""
	}
	if len(catalog.Issues) > 0 {
		return catalog, fmt.Sprintf(
			"found %d invalid invariant definitions; run argus invariant inspect",
			len(catalog.Issues),
		)
	}

	return catalog, ""
}

func runTickInvariants(ctx context.Context, catalog *invariant.Catalog, projectRoot string, firstTick bool) tickInvariantRun {
	run := tickInvariantRun{}
	if catalog == nil {
		return run
	}

	if len(catalog.Invariants) == 0 {
		return run
	}

	for _, inv := range catalog.Invariants {
		if !shouldRunInvariantAuto(inv, firstTick) {
			continue
		}

		result := invariant.RunCheck(ctx, inv, projectRoot)
		run.RanChecks++
		run.TotalTime += result.TotalTime
		if result.Passed {
			continue
		}

		run.Failure = buildInvariantFailure(inv, result)
		return run
	}

	return run
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

func buildInvariantFailure(inv *invariant.Invariant, result *invariant.CheckResult) *InvariantFailure {
	if inv == nil {
		return nil
	}

	return &InvariantFailure{
		Invariant:  inv,
		FailedStep: firstFailedInvariantStep(result),
	}
}

func firstFailedInvariantStep(result *invariant.CheckResult) *invariant.StepResult {
	if result == nil {
		return nil
	}

	for i := range result.Steps {
		if result.Steps[i].Status != "fail" {
			continue
		}
		return &result.Steps[i]
	}

	return nil
}

func activePipelineIDs(activePipelines []pipeline.ActivePipeline) []string {
	if len(activePipelines) == 0 {
		return nil
	}

	instanceIDs := make([]string, 0, len(activePipelines))
	for _, active := range activePipelines {
		instanceIDs = append(instanceIDs, active.InstanceID)
	}
	return instanceIDs
}

func writeTickWarningf(stdout io.Writer, format string, args ...any) {
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

func formatSlowInvariantWarning(totalCheckTime time.Duration) string {
	return fmt.Sprintf(
		"Invariant checks took %.1fs total. Use the `argus-doctor` skill to assess invariant risk before running `argus doctor --check-invariants`.",
		totalCheckTime.Seconds(),
	)
}
