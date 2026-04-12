package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

type workflowStartOutput struct {
	PipelineStatus string               `json:"pipeline_status"`
	Progress       string               `json:"progress"`
	NextJob        workflowStartNextJob `json:"next_job"`
}

type workflowStartNextJob struct {
	ID     string  `json:"id"`
	Prompt string  `json:"prompt"`
	Skill  *string `json:"skill"`
}

// SEQUENCE-TEST: output consumed by job-done, status, cancel — see cmd_pipeline_lifecycle_test.go
func newWorkflowStartCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "start <workflow-id>",
		Short: "Start a workflow pipeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			if err := core.ValidateWorkflowID(workflowID); err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow start failed: %w", err)
			}

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

			w, err := s.Artifacts().Workflows().Load(workflowID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow start failed: %w", err)
			}

			p, instanceID, err := s.Artifacts().Pipelines().Create(workflowID, w, time.Now())
			if err != nil {
				msg := err.Error()
				if errors.Is(err, core.ErrActivePipelineExists) {
					msg = "Another pipeline is already running. Complete or cancel the current pipeline before starting a new one."
				}
				writeCommandError(cmd, jsonFlag, msg)
				return fmt.Errorf("workflow start failed: %w", err)
			}

			templateJobs := make(map[string]*workflow.PipelineJobData, len(p.Jobs))
			for id, jd := range p.Jobs {
				templateJobs[id] = &workflow.PipelineJobData{
					StartedAt: jd.StartedAt,
					EndedAt:   jd.EndedAt,
					Message:   jd.Message,
				}
			}

			templateCtx := workflow.BuildContext(cmd.Context(), templateJobs, w, 0)
			firstJob := w.Jobs[0]
			rendered, warnings := workflow.RenderPrompt(firstJob.Prompt, templateCtx)

			for _, warning := range warnings {
				_, _ = fmt.Fprintf(os.Stderr, "Argus warning: %s\n", warning)
			}

			progress := fmt.Sprintf("1/%d", len(w.Jobs))

			var skill *string
			if firstJob.Skill != "" {
				skillVal := firstJob.Skill
				skill = &skillVal
			}

			outData := workflowStartOutput{
				PipelineStatus: "running",
				Progress:       progress,
				NextJob: workflowStartNextJob{
					ID:     firstJob.ID,
					Prompt: rendered,
					Skill:  skill,
				},
			}

			if jsonFlag {
				return writeJSONOK(cmd, outData)
			}

			renderStartText(cmd, instanceID, progress, firstJob, rendered)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderStartText(cmd *cobra.Command, instanceID, progress string, job workflow.Job, renderedPrompt string) {
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Argus: Pipeline %s started (%s)\n\n", instanceID, progress)
	_, _ = fmt.Fprintf(w, "Current job: %s\n", job.ID)
	_, _ = fmt.Fprintf(w, "Prompt: %s\n", renderedPrompt)
	if job.Skill != "" {
		_, _ = fmt.Fprintf(w, "Skill: %s\n", job.Skill)
	}
	_, _ = fmt.Fprintf(w, "\nWhen complete, run: argus job-done --message \"execution summary\"\n")
}
