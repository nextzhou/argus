// Package hook provides hook command handlers for multi-agent integration.
package hook

import (
	"fmt"
	"strings"
)

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

// FormatNoPipeline returns Markdown text listing available workflows when no pipeline is active.
// It includes the [Argus] marker, workflow list, and the start command.
func FormatNoPipeline(workflows []WorkflowSummary) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[Argus] No active pipeline.\n\n")
	fmt.Fprintf(&buf, "Available workflows:\n")

	if len(workflows) == 0 {
		fmt.Fprintf(&buf, "  (none)\n")
	} else {
		for _, w := range workflows {
			fmt.Fprintf(&buf, "  - %s: %s\n", w.ID, w.Description)
		}
	}

	fmt.Fprintf(&buf, "\nTo start: argus workflow start <workflow-id>\n")
	return buf.String()
}

// FormatFullContext returns Markdown text with complete job context including all action commands.
// It includes pipeline ID, workflow ID, progress, job ID, prompt, skill (if non-empty),
// and action commands for job-done, snooze, and cancel.
func FormatFullContext(pipelineID, workflowID, progress, jobID, prompt, skill, sessionID string) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[Argus] Pipeline: %s | Workflow: %s | Progress: %s\n\n", pipelineID, workflowID, progress)
	fmt.Fprintf(&buf, "Current Job: %s\n", jobID)

	if skill != "" {
		fmt.Fprintf(&buf, "Skill: %s\n", skill)
	}

	fmt.Fprintf(&buf, "\n%s\n\n", prompt)
	fmt.Fprintf(&buf, "When done: argus job-done [--message \"summary\"]\n")
	fmt.Fprintf(&buf, "To snooze: argus workflow snooze --session %s\n", sessionID)
	fmt.Fprintf(&buf, "To cancel: argus workflow cancel\n")

	return buf.String()
}

// FormatMinimalSummary returns a short Markdown reminder when pipeline state hasn't changed.
// It includes workflow ID, job ID, progress, and the job-done command.
func FormatMinimalSummary(workflowID, jobID, progress string) string {
	return fmt.Sprintf("[Argus] %s | Job: %s | Progress: %s — When done: argus job-done\n", workflowID, jobID, progress)
}

// FormatSnoozed returns Markdown text for a snoozed pipeline.
// A snoozed pipeline is invisible to the user, so this returns the same output as FormatNoPipeline.
func FormatSnoozed(workflows []WorkflowSummary) string {
	return FormatNoPipeline(workflows)
}

// AppendInvariantFailed appends an invariant failure section to the base output.
// If failures is empty, the base output is returned unchanged.
// The appended section includes the [Argus] marker and a list of failed invariants with suggestions.
func AppendInvariantFailed(base string, failures []InvariantFailure) string {
	if len(failures) == 0 {
		return base
	}

	var buf strings.Builder
	buf.WriteString(base)
	buf.WriteString("\n---\n")
	buf.WriteString("[Argus] Invariant check failed:\n")

	for _, f := range failures {
		fmt.Fprintf(&buf, "  - %s: %s\n", f.ID, f.Description)
		fmt.Fprintf(&buf, "    Suggestion: %s\n", f.Suggestion)
	}

	return buf.String()
}
