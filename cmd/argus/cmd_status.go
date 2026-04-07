package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/spf13/cobra"
)

type statusOutput struct {
	Pipeline   *statusPipeline  `json:"pipeline"`
	Invariants statusInvariants `json:"invariants"`
	Hints      []string         `json:"hints"`
}

type statusPipeline struct {
	WorkflowID string         `json:"workflow_id"`
	Status     string         `json:"status"`
	CurrentJob *string        `json:"current_job"`
	StartedAt  string         `json:"started_at"`
	EndedAt    *string        `json:"ended_at"`
	Progress   statusProgress `json:"progress"`
	Jobs       []statusJob    `json:"jobs"`
}

type statusProgress struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

type statusJob struct {
	ID      string  `json:"id"`
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

type statusInvariantDetail struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type statusInvariants struct {
	Passed  int                     `json:"passed"`
	Failed  int                     `json:"failed"`
	Details []statusInvariantDetail `json:"details"`
}

func newStatusCmd() *cobra.Command {
	var markdownFlag bool

	cmd := &cobra.Command{
		Use:    "status",
		Short:  "Show project status",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pipelinesDir := filepath.Join(".argus", "pipelines")

			actives, _, err := pipeline.ScanActivePipelines(pipelinesDir)
			if err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("status failed: %w", err)
			}

			if len(actives) > 1 {
				msg := "检测到多个活跃的 Pipeline（异常状态）。请运行 argus doctor 排查。"
				errBytes, _ := core.ErrorEnvelope(msg)
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("status failed: multiple active pipelines")
			}

			out := statusOutput{
				Invariants: statusInvariants{
					Details: []statusInvariantDetail{},
				},
				Hints: []string{},
			}

			runStatusInvariants(&out)

			if len(actives) == 0 {
				if markdownFlag {
					renderStatusMarkdownNoPipeline(cmd.OutOrStdout(), out)
					return nil
				}
				outBytes, marshalErr := core.OKEnvelope(out)
				if marshalErr != nil {
					return fmt.Errorf("marshaling output: %w", marshalErr)
				}
				_, _ = os.Stdout.Write(outBytes)
				_, _ = os.Stdout.WriteString("\n")
				return nil
			}

			active := actives[0]
			p := active.Pipeline
			instanceID := active.InstanceID

			workflowsDir := filepath.Join(".argus", "workflows")
			workflowPath := filepath.Join(workflowsDir, p.WorkflowID+".yaml")

			wf, err := workflow.ParseWorkflowFile(workflowPath)
			if err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("status failed: %w", err)
			}

			if err := resolveRefs(workflowsDir, workflowPath, wf); err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("status failed: %w", err)
			}

			sp := buildStatusPipeline(p, wf, instanceID, &out)

			out.Pipeline = sp

			if markdownFlag {
				renderStatusMarkdownActive(cmd.OutOrStdout(), out, instanceID)
				return nil
			}

			outBytes, marshalErr := core.OKEnvelope(out)
			if marshalErr != nil {
				return fmt.Errorf("marshaling output: %w", marshalErr)
			}
			_, _ = os.Stdout.Write(outBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&markdownFlag, "markdown", false, "Output human-readable markdown summary")
	return cmd
}

func buildStatusPipeline(p *pipeline.Pipeline, wf *workflow.Workflow, _ string, out *statusOutput) *statusPipeline {
	sp := &statusPipeline{
		WorkflowID: p.WorkflowID,
		Status:     p.Status,
		CurrentJob: p.CurrentJob,
		StartedAt:  p.StartedAt,
		EndedAt:    p.EndedAt,
		Progress: statusProgress{
			Total: len(wf.Jobs),
		},
	}

	if p.CurrentJob != nil {
		_, found := pipeline.FindJobIndex(wf, *p.CurrentJob)
		if !found {
			sp.CurrentJob = nil
			out.Hints = append(out.Hints, "当前 job 在 workflow 定义中未找到，可能 workflow 定义已变更。")
		} else {
			idx, _ := pipeline.FindJobIndex(wf, *p.CurrentJob)
			sp.Progress.Current = idx + 1
		}
	}

	sp.Jobs = make([]statusJob, 0, len(wf.Jobs))
	for _, job := range wf.Jobs {
		sj := statusJob{
			ID:     job.ID,
			Status: pipeline.DeriveJobStatus(p, wf, job.ID),
		}
		if jd, ok := p.Jobs[job.ID]; ok {
			sj.Message = jd.Message
		}
		sp.Jobs = append(sp.Jobs, sj)
	}

	return sp
}

func runStatusInvariants(out *statusOutput) {
	invariantsDir := filepath.Join(".argus", "invariants")
	entries, err := os.ReadDir(invariantsDir)
	if err != nil {
		return
	}

	var totalCheckTime time.Duration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		fullPath := filepath.Join(invariantsDir, entry.Name())
		inv, parseErr := invariant.ParseInvariantFile(fullPath)
		if parseErr != nil {
			continue
		}

		if inv.Auto == "never" {
			continue
		}

		result := invariant.RunCheck(context.Background(), inv, ".")
		totalCheckTime += result.TotalTime

		status := "passed"
		if !result.Passed {
			status = "failed"
			out.Invariants.Failed++
		} else {
			out.Invariants.Passed++
		}

		out.Invariants.Details = append(out.Invariants.Details, statusInvariantDetail{
			ID:          inv.ID,
			Description: invariantDescription(inv),
			Status:      status,
		})
	}

	if totalCheckTime.Seconds() > 2 {
		out.Hints = append(out.Hints, fmt.Sprintf("Invariant 检查总耗时 %.1fs，建议运行 argus doctor 排查慢检查项", totalCheckTime.Seconds()))
	}
}

func renderStatusMarkdownFailedInvariants(w io.Writer, out statusOutput) {
	for _, d := range out.Invariants.Details {
		if d.Status == "failed" {
			_, _ = fmt.Fprintf(w, "  [FAIL] %s: %s\n", d.ID, d.Description)
		}
	}
}

func renderStatusMarkdownNoPipeline(w io.Writer, out statusOutput) {
	_, _ = fmt.Fprintf(w, "[Argus] 项目状态\n\n")
	_, _ = fmt.Fprintf(w, "Pipeline: 无活跃 Pipeline\n\n")
	_, _ = fmt.Fprintf(w, "Invariant: %d passed, %d failed\n", out.Invariants.Passed, out.Invariants.Failed)
	renderStatusMarkdownFailedInvariants(w, out)
}

func renderStatusMarkdownActive(w io.Writer, out statusOutput, instanceID string) {
	_, _ = fmt.Fprintf(w, "[Argus] 项目状态\n\n")

	sp := out.Pipeline
	_, _ = fmt.Fprintf(w, "Pipeline: %s (%s) - Workflow: %s - 进度 %d/%d\n",
		instanceID, sp.Status, sp.WorkflowID, sp.Progress.Current, sp.Progress.Total)

	for i, job := range sp.Jobs {
		marker := "[ ]"
		switch job.Status {
		case "completed":
			marker = "[done]"
		case "in_progress":
			marker = "[>>]  "
		case "pending":
			marker = "[ ]   "
		}

		if job.Message != nil {
			_, _ = fmt.Fprintf(w, "  %d. %s %s - %s\n", i+1, marker, job.ID, *job.Message)
		} else {
			_, _ = fmt.Fprintf(w, "  %d. %s %s\n", i+1, marker, job.ID)
		}
	}

	_, _ = fmt.Fprintf(w, "\nInvariant: %d passed, %d failed\n", out.Invariants.Passed, out.Invariants.Failed)
	renderStatusMarkdownFailedInvariants(w, out)
}
