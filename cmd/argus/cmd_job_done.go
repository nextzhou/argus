package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

type jobDoneOutput struct {
	PipelineStatus string       `json:"pipeline_status"`
	Progress       string       `json:"progress"`
	NextJob        *nextJobInfo `json:"next_job"`
	EarlyExit      *bool        `json:"early_exit,omitempty"`
	FailedJob      *string      `json:"failed_job,omitempty"`
}

type nextJobInfo struct {
	ID     string  `json:"id"`
	Prompt string  `json:"prompt"`
	Skill  *string `json:"skill"`
}

// SEQUENCE-TEST: output consumed by status; consumes state from workflow start — see cmd_pipeline_lifecycle_test.go
func newJobDoneCmd() *cobra.Command {
	var (
		failFlag        bool
		endPipelineFlag bool
		messageFlag     string
		jsonFlag        bool
	)

	cmd := &cobra.Command{
		Use:    "job-done",
		Short:  "Mark the current job as done",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			s, err := scope.ResolveScope(cwd)
			if err != nil {
				return fmt.Errorf("resolving scope: %w", err)
			}
			if s == nil {
				return fmt.Errorf("not inside an Argus project or registered workspace")
			}

			actives, _, err := s.Artifacts().Pipelines().ScanActive()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("job-done failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "No active pipeline. Start one with argus workflow start <workflow-id>."
				if jsonFlag {
					writeCommandError(cmd, true, msg)
				} else {
					renderNoPipelineText(cmd.ErrOrStderr())
				}
				return fmt.Errorf("job-done failed: %w", core.ErrNoActivePipeline)
			}

			if len(actives) > 1 {
				msg := "Detected multiple active pipelines (unexpected state)."
				writeCommandError(cmd, jsonFlag, msg)
				return fmt.Errorf("job-done failed: multiple active pipelines")
			}

			active := actives[0]
			p := active.Pipeline
			instanceID := active.InstanceID

			wf, err := s.Artifacts().Workflows().Load(p.WorkflowID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("job-done failed: %w", err)
			}

			completedJobID := *p.CurrentJob

			var msgPtr *string
			if cmd.Flags().Changed("message") {
				msgPtr = &messageFlag
			}
			opts := pipeline.AdvanceOpts{
				Fail:        failFlag,
				EndPipeline: endPipelineFlag,
				Message:     msgPtr,
				Now:         time.Now(),
			}

			if err := pipeline.AdvanceJob(p, wf, opts); err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("job-done failed: %w", err)
			}

			if err := s.Artifacts().Pipelines().Save(instanceID, p); err != nil {
				return fmt.Errorf("saving pipeline: %w", err)
			}

			completedJobIdx, _ := pipeline.FindJobIndex(wf, completedJobID)
			progress := fmt.Sprintf("%d/%d", completedJobIdx+1, len(wf.Jobs))

			var renderedPrompt string
			var nextJob workflow.Job
			if p.Status == pipeline.StatusRunning && p.CurrentJob != nil {
				nextJobIdx, _ := pipeline.FindJobIndex(wf, *p.CurrentJob)
				nextJob = wf.Jobs[nextJobIdx]

				templateJobs := make(map[string]*workflow.PipelineJobData, len(p.Jobs))
				for id, jd := range p.Jobs {
					templateJobs[id] = &workflow.PipelineJobData{
						StartedAt: jd.StartedAt,
						EndedAt:   jd.EndedAt,
						Message:   jd.Message,
					}
				}

				templateCtx := workflow.BuildContext(cmd.Context(), templateJobs, wf, nextJobIdx)
				rendered, warnings := workflow.RenderPrompt(nextJob.Prompt, templateCtx)
				renderedPrompt = rendered
				for _, w := range warnings {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Argus warning: %s\n", w)
				}
			}

			out := jobDoneOutput{
				PipelineStatus: p.Status,
				Progress:       progress,
			}

			if failFlag {
				failedJob := completedJobID
				out.FailedJob = &failedJob
			}
			if endPipelineFlag {
				earlyExit := true
				out.EarlyExit = &earlyExit
			}
			if p.Status == pipeline.StatusRunning && p.CurrentJob != nil {
				var skill *string
				if nextJob.Skill != "" {
					skillVal := nextJob.Skill
					skill = &skillVal
				}
				out.NextJob = &nextJobInfo{
					ID:     nextJob.ID,
					Prompt: renderedPrompt,
					Skill:  skill,
				}
			}

			if jsonFlag {
				return writeJSONOK(cmd, out)
			}

			w := cmd.OutOrStdout()
			switch {
			case failFlag:
				renderFailedText(w, completedJobID, progress, wf.ID, endPipelineFlag)
			case p.Status == pipeline.StatusCompleted && endPipelineFlag:
				renderEarlyExitText(w, completedJobID, progress)
			case p.Status == pipeline.StatusCompleted:
				renderCompletedText(w, completedJobID, progress, instanceID)
			case p.Status == pipeline.StatusRunning:
				renderNextJobText(w, completedJobID, progress, nextJob, renderedPrompt)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&failFlag, "fail", false, "Mark the current job as failed")
	cmd.Flags().BoolVar(&endPipelineFlag, "end-pipeline", false, "End the pipeline early")
	cmd.Flags().StringVar(&messageFlag, "message", "", "Message to record with the job completion")
	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderNoPipelineText(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Argus: No active pipeline.\n")
	_, _ = fmt.Fprintf(w, "Start one with argus workflow start <workflow-id>.\n")
}

func renderNextJobText(w io.Writer, completedJobID, progress string, nextJob workflow.Job, renderedPrompt string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s completed (%s)\n\n", completedJobID, progress)
	_, _ = fmt.Fprintf(w, "Next job: %s\n", nextJob.ID)
	_, _ = fmt.Fprintf(w, "Prompt: %s\n", renderedPrompt)
	if nextJob.Skill != "" {
		_, _ = fmt.Fprintf(w, "Skill: %s\n", nextJob.Skill)
	}
	_, _ = fmt.Fprintf(w, "\nWhen complete, run: argus job-done --message \"execution summary\"\n")
}

func renderCompletedText(w io.Writer, completedJobID, progress, instanceID string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s completed (%s)\n", completedJobID, progress)
	_, _ = fmt.Fprintf(w, "Pipeline %s is complete.\n", instanceID)
}

func renderEarlyExitText(w io.Writer, completedJobID, progress string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s completed. Pipeline ended early (%s).\n", completedJobID, progress)
}

func renderFailedText(w io.Writer, failedJobID, progress, workflowID string, earlyExit bool) {
	if earlyExit {
		_, _ = fmt.Fprintf(w, "Argus: Job %s marked as failed. Pipeline ended early (%s).\n", failedJobID, progress)
	} else {
		_, _ = fmt.Fprintf(w, "Argus: Job %s marked as failed. Pipeline stopped (%s).\n", failedJobID, progress)
	}
	_, _ = fmt.Fprintf(w, "\nAvailable actions:\n")
	_, _ = fmt.Fprintf(w, "- Restart: argus workflow start %s\n", workflowID)
	_, _ = fmt.Fprintf(w, "- Cancel: argus workflow cancel\n")
}
