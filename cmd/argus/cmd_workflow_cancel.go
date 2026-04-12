package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/spf13/cobra"
)

type workflowCancelOutput struct {
	Cancelled []string `json:"cancelled"`
}

// SEQUENCE-TEST: consumes state from workflow start — see cmd_pipeline_lifecycle_test.go
func newWorkflowCancelCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel the active pipeline",
		Args:  cobra.NoArgs,
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
				return fmt.Errorf("workflow cancel failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "No active pipeline."
				writeCommandError(cmd, jsonFlag, msg)
				return fmt.Errorf("workflow cancel failed: %w", core.ErrNoActivePipeline)
			}

			now := time.Now()
			cancelled := make([]string, 0, len(actives))

			for _, active := range actives {
				pipeline.CancelPipeline(active.Pipeline, now)
				if saveErr := s.Artifacts().Pipelines().Save(active.InstanceID, active.Pipeline); saveErr != nil {
					return fmt.Errorf("saving cancelled pipeline %q: %w", active.InstanceID, saveErr)
				}
				cancelled = append(cancelled, active.InstanceID)
			}

			out := workflowCancelOutput{Cancelled: cancelled}
			if jsonFlag {
				return writeJSONOK(cmd, out)
			}

			renderWorkflowCancelText(cmd.OutOrStdout(), cancelled)
			return nil
		},
	}

	bindJSONFlag(cmd, &jsonFlag)
	return cmd
}

func renderWorkflowCancelText(w io.Writer, cancelled []string) {
	_, _ = fmt.Fprintf(w, "Argus: Cancelled %d pipeline(s).\n", len(cancelled))
	for _, instanceID := range cancelled {
		_, _ = fmt.Fprintf(w, "- %s\n", instanceID)
	}
}
