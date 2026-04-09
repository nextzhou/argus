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

			actives, _, err := s.ScanActivePipelines()
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("job-done failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "当前没有活跃的 Pipeline。可以使用 argus workflow start <workflow-id> 启动一个 workflow。"
				if jsonFlag {
					writeCommandError(cmd, true, msg)
				} else {
					renderNoPipelineText(cmd.ErrOrStderr())
				}
				return fmt.Errorf("job-done failed: %w", core.ErrNoActivePipeline)
			}

			if len(actives) > 1 {
				msg := "检测到多个活跃的 Pipeline（异常状态）。"
				writeCommandError(cmd, jsonFlag, msg)
				return fmt.Errorf("job-done failed: multiple active pipelines")
			}

			active := actives[0]
			p := active.Pipeline
			instanceID := active.InstanceID

			wf, err := s.LoadWorkflow(p.WorkflowID)
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

			if err := pipeline.SavePipeline(s.PipelinesDir(), instanceID, p); err != nil {
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

				ctx := workflow.BuildContext(templateJobs, wf, nextJobIdx)
				rendered, warnings := workflow.RenderPrompt(nextJob.Prompt, ctx)
				renderedPrompt = rendered
				for _, w := range warnings {
					_, _ = fmt.Fprintf(os.Stderr, "Argus warning: %s\n", w)
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
	_, _ = fmt.Fprintf(w, "Argus: 当前没有活跃的 Pipeline。\n")
	_, _ = fmt.Fprintf(w, "可以使用 argus workflow start <workflow-id> 启动一个 workflow。\n")
}

func renderNextJobText(w io.Writer, completedJobID, progress string, nextJob workflow.Job, renderedPrompt string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s 完成 (%s)\n\n", completedJobID, progress)
	_, _ = fmt.Fprintf(w, "下一个 Job: %s\n", nextJob.ID)
	_, _ = fmt.Fprintf(w, "Prompt: %s\n", renderedPrompt)
	if nextJob.Skill != "" {
		_, _ = fmt.Fprintf(w, "Skill: %s\n", nextJob.Skill)
	}
	_, _ = fmt.Fprintf(w, "\n完成后请调用：argus job-done --message \"执行结果摘要\"\n")
}

func renderCompletedText(w io.Writer, completedJobID, progress, instanceID string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s 完成 (%s)\n", completedJobID, progress)
	_, _ = fmt.Fprintf(w, "Pipeline %s 已全部完成。\n", instanceID)
}

func renderEarlyExitText(w io.Writer, completedJobID, progress string) {
	_, _ = fmt.Fprintf(w, "Argus: Job %s 完成，Pipeline 提前结束 (%s)。\n", completedJobID, progress)
}

func renderFailedText(w io.Writer, failedJobID, progress, workflowID string, earlyExit bool) {
	if earlyExit {
		_, _ = fmt.Fprintf(w, "Argus: Job %s 标记为失败，Pipeline 提前结束 (%s)。\n", failedJobID, progress)
	} else {
		_, _ = fmt.Fprintf(w, "Argus: Job %s 标记为失败，Pipeline 已停止 (%s)。\n", failedJobID, progress)
	}
	_, _ = fmt.Fprintf(w, "\n可用操作：\n")
	_, _ = fmt.Fprintf(w, "- 重新开始：argus workflow start %s\n", workflowID)
	_, _ = fmt.Fprintf(w, "- 取消：argus workflow cancel\n")
}
