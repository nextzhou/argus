// Package hook provides hook command handlers for multi-agent integration.
package hook

import "github.com/nextzhou/argus/internal/invariant"

// WorkflowSummary contains minimal workflow information for tick output.
type WorkflowSummary struct {
	ID          string
	Description string
}

// InvariantFailure contains information about a failed invariant check.
type InvariantFailure struct {
	Invariant  *invariant.Invariant
	FailedStep *invariant.StepResult
}

// ActivePipelineIssue captures one active-pipeline anomaly that needs user input.
type ActivePipelineIssue struct {
	PipelineID          string
	WorkflowID          string
	Issue               string
	InvestigateCommand  string
	InvestigateGuidance string
	SessionID           string
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

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
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

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
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

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
	return renderTemplate("prompts/tick-minimal.md.tmpl", data)
}

// FormatSnoozed returns readable text for a snoozed pipeline.
func FormatSnoozed(workflows []WorkflowSummary) (string, error) {
	data := struct {
		Workflows []WorkflowSummary
	}{
		Workflows: workflows,
	}

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
	return renderTemplate("prompts/tick-snoozed.md.tmpl", data)
}

// FormatInvariantFailure returns readable text for a failed invariant when no
// active pipeline is running. Invariant failures are a mutually exclusive tick
// outcome and do not append to other output.
func FormatInvariantFailure(failure InvariantFailure) (string, error) {
	data := struct {
		Failure InvariantFailure
	}{
		Failure: failure,
	}
	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
	return renderTemplate("prompts/tick-invariant-failed.md.tmpl", data)
}

// FormatActivePipelineIssue returns guidance for one active-pipeline anomaly.
func FormatActivePipelineIssue(issue ActivePipelineIssue) (string, error) {
	data := struct {
		Issue ActivePipelineIssue
	}{
		Issue: issue,
	}

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
	return renderTemplate("prompts/tick-active-pipeline-issue.md.tmpl", data)
}

// FormatMultipleActivePipelines returns guidance for the multi-active anomaly.
func FormatMultipleActivePipelines(instanceIDs []string, sessionID string) (string, error) {
	data := struct {
		InstanceIDs []string
		SessionID   string
	}{
		InstanceIDs: instanceIDs,
		SessionID:   sessionID,
	}

	// Keep docs/technical-tick.md in sync with user-visible contract changes in
	// this template or its injected fields.
	return renderTemplate("prompts/tick-multiple-active-pipelines.md.tmpl", data)
}
