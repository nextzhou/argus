package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
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
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:    "status",
		Short:  "Show project status",
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
				return fmt.Errorf("status failed: %w", err)
			}

			if len(actives) > 1 {
				msg := "检测到多个活跃的 Pipeline（异常状态）。请运行 argus doctor 排查。"
				writeCommandError(cmd, jsonFlag, msg)
				return fmt.Errorf("status failed: multiple active pipelines")
			}

			out := statusOutput{
				Invariants: statusInvariants{
					Details: []statusInvariantDetail{},
				},
				Hints: []string{},
			}

			runStatusInvariants(cmd.Context(), s, &out)

			if len(actives) == 0 {
				if jsonFlag {
					return writeJSONOK(cmd, out)
				}

				renderStatusTextNoPipeline(cmd.OutOrStdout(), out)
				return nil
			}

			active := actives[0]
			p := active.Pipeline
			instanceID := active.InstanceID

			wf, err := s.LoadWorkflow(p.WorkflowID)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("status failed: %w", err)
			}

			sp := buildStatusPipeline(p, wf, instanceID, &out)

			out.Pipeline = sp

			if jsonFlag {
				return writeJSONOK(cmd, out)
			}

			renderStatusTextActive(cmd.OutOrStdout(), out, instanceID)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
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

func runStatusInvariants(ctx context.Context, s scope.Scope, out *statusOutput) {
	invs, err := s.LoadInvariants()
	if err != nil {
		return
	}

	var totalCheckTime time.Duration
	for _, inv := range invs {
		if inv.Auto == "never" {
			continue
		}

		result := invariant.RunCheck(ctx, inv, s.ProjectRoot())
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

func renderStatusTextFailedInvariants(w io.Writer, out statusOutput) {
	for _, d := range out.Invariants.Details {
		if d.Status == "failed" {
			_, _ = fmt.Fprintf(w, "  [FAIL] %s: %s\n", d.ID, d.Description)
		}
	}
}

func renderStatusTextNoPipeline(w io.Writer, out statusOutput) {
	_, _ = fmt.Fprintf(w, "Argus: 项目状态\n\n")
	_, _ = fmt.Fprintf(w, "Pipeline: 无活跃 Pipeline\n\n")
	_, _ = fmt.Fprintf(w, "Invariant: %d passed, %d failed\n", out.Invariants.Passed, out.Invariants.Failed)
	renderStatusTextFailedInvariants(w, out)
}

func renderStatusTextActive(w io.Writer, out statusOutput, instanceID string) {
	_, _ = fmt.Fprintf(w, "Argus: 项目状态\n\n")

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
	renderStatusTextFailedInvariants(w, out)
}
