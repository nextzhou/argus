package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/spf13/cobra"
)

type workflowCancelOutput struct {
	Cancelled []string `json:"cancelled"`
}

func newWorkflowCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel",
		Short: "Cancel the active pipeline",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			pipelinesDir := filepath.Join(".argus", "pipelines")

			actives, _, err := pipeline.ScanActivePipelines(pipelinesDir)
			if err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("workflow cancel failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "当前没有活跃的 Pipeline。"
				errBytes, _ := core.ErrorEnvelope(msg)
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("workflow cancel failed: %w", core.ErrNoActivePipeline)
			}

			now := time.Now()
			cancelled := make([]string, 0, len(actives))

			for _, active := range actives {
				pipeline.CancelPipeline(active.Pipeline, now)
				if saveErr := pipeline.SavePipeline(pipelinesDir, active.InstanceID, active.Pipeline); saveErr != nil {
					return fmt.Errorf("saving cancelled pipeline %q: %w", active.InstanceID, saveErr)
				}
				cancelled = append(cancelled, active.InstanceID)
			}

			outBytes, err := core.OKEnvelope(workflowCancelOutput{Cancelled: cancelled})
			if err != nil {
				return fmt.Errorf("marshaling output: %w", err)
			}
			_, _ = os.Stdout.Write(outBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}
}
