package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

			workflowsDir := filepath.Join(".argus", "workflows")
			workflowPath := filepath.Join(workflowsDir, workflowID+".yaml")
			pipelinesDir := filepath.Join(".argus", "pipelines")

			w, err := workflow.ParseWorkflowFile(workflowPath)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow start failed: %w", err)
			}

			if err := resolveRefs(workflowsDir, workflowPath, w); err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow start failed: %w", err)
			}

			p, instanceID, err := pipeline.CreatePipeline(pipelinesDir, workflowID, w, time.Now())
			if err != nil {
				msg := err.Error()
				if errors.Is(err, core.ErrActivePipelineExists) {
					msg = "另一个 Pipeline 正在运行中，请先完成或取消当前 Pipeline 后再启动新的 Pipeline。"
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

			ctx := workflow.BuildContext(templateJobs, w, 0)
			firstJob := w.Jobs[0]
			rendered, warnings := workflow.RenderPrompt(firstJob.Prompt, ctx)

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
				PipelineStatus: pipeline.StatusRunning,
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
	_, _ = fmt.Fprintf(w, "Argus: Pipeline %s 已启动 (%s)\n\n", instanceID, progress)
	_, _ = fmt.Fprintf(w, "当前 Job: %s\n", job.ID)
	_, _ = fmt.Fprintf(w, "Prompt: %s\n", renderedPrompt)
	if job.Skill != "" {
		_, _ = fmt.Fprintf(w, "Skill: %s\n", job.Skill)
	}
	_, _ = fmt.Fprintf(w, "\n完成后请调用：argus job-done --message \"执行结果摘要\"\n")
}

func resolveRefs(workflowsDir, workflowPath string, w *workflow.Workflow) error {
	hasRefs := false
	for _, job := range w.Jobs {
		if job.Ref != "" {
			hasRefs = true
			break
		}
	}
	if !hasRefs {
		return nil
	}

	sharedPath := filepath.Join(workflowsDir, "_shared.yaml")
	shared, err := workflow.LoadShared(sharedPath)
	if err != nil {
		return fmt.Errorf("loading shared definitions: %w", err)
	}

	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return fmt.Errorf("re-reading workflow for ref resolution: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing workflow nodes: %w", err)
	}

	jobNodes := findJobNodes(&doc)
	for i, job := range w.Jobs {
		if job.Ref == "" || i >= len(jobNodes) {
			continue
		}
		resolved, err := workflow.ResolveRef(jobNodes[i], shared)
		if err != nil {
			return fmt.Errorf("resolving ref for job[%d]: %w", i, err)
		}
		w.Jobs[i] = *resolved
	}

	return nil
}

func findJobNodes(doc *yaml.Node) []*yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "jobs" {
			jobsNode := root.Content[i+1]
			if jobsNode.Kind != yaml.SequenceNode {
				return nil
			}
			return jobsNode.Content
		}
	}

	return nil
}
