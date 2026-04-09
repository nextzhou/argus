// Package hook provides hook command handlers for multi-agent integration.
package hook

// WorkflowSummary contains minimal workflow information for tick output.
type WorkflowSummary struct {
	ID          string
	Description string
}

// InvariantFailure contains information about a failed invariant check.
type InvariantFailure struct {
	ID          string
	Description string
	Suggestion  string
}

// FormatNoPipeline returns readable text listing available workflows when no pipeline is active.
// Tick output must stay plain text and must not start with '[' or '{', because
// Codex treats those prefixes as candidate JSON in UserPromptSubmit hooks.
// All agents therefore share the same "Argus:" text prefix.
func FormatNoPipeline(workflows []WorkflowSummary) (string, error) {
	data := struct {
		Workflows []WorkflowSummary
	}{
		Workflows: workflows,
	}

	return renderTemplate("prompts/tick-no-pipeline.md.tmpl", data)
}

// FormatFullContext returns readable text with complete job context including all action commands.
// It includes pipeline ID, workflow ID, progress, job ID, prompt, skill (if non-empty),
// and action commands for job-done, snooze, and cancel.
func FormatFullContext(pipelineID, workflowID, progress, jobID, prompt, skill, sessionID string) (string, error) {
	data := struct {
		PipelineID string
		WorkflowID string
		Progress   string
		JobID      string
		Prompt     string
		Skill      string
		SessionID  string
	}{
		PipelineID: pipelineID,
		WorkflowID: workflowID,
		Progress:   progress,
		JobID:      jobID,
		Prompt:     prompt,
		Skill:      skill,
		SessionID:  sessionID,
	}

	return renderTemplate("prompts/tick-full-context.md.tmpl", data)
}

// FormatMinimalSummary returns a short readable reminder when pipeline state hasn't changed.
// It includes workflow ID, job ID, progress, and the job-done command.
func FormatMinimalSummary(workflowID, jobID, progress string) (string, error) {
	data := struct {
		WorkflowID string
		JobID      string
		Progress   string
	}{
		WorkflowID: workflowID,
		JobID:      jobID,
		Progress:   progress,
	}

	return renderTemplate("prompts/tick-minimal.md.tmpl", data)
}

// FormatSnoozed returns readable text for a snoozed pipeline.
// A snoozed pipeline is invisible to the user, so this returns the same output as FormatNoPipeline.
func FormatSnoozed(workflows []WorkflowSummary) (string, error) {
	return FormatNoPipeline(workflows)
}

// AppendInvariantFailed appends an invariant failure section to the base output.
// If failures is empty, the base output is returned unchanged.
// The appended section preserves the same hook-safe "Argus:" text prefix and
// lists failed invariants with suggestions.
func AppendInvariantFailed(base string, failures []InvariantFailure) (string, error) {
	if len(failures) == 0 {
		return base, nil
	}

	data := struct {
		Failures []InvariantFailure
	}{
		Failures: failures,
	}
	appendix, err := renderTemplate("prompts/tick-invariant-failed.md.tmpl", data)
	if err != nil {
		return "", err
	}

	return base + "\n" + appendix, nil
}
