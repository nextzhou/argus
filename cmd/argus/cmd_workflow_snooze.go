package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/session"
	"github.com/spf13/cobra"
)

type workflowSnoozeOutput struct {
	Snoozed []string `json:"snoozed"`
}

func newWorkflowSnoozeCmd() *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "snooze",
		Short: "Snooze active pipelines for a session",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			pipelinesDir := filepath.Join(".argus", "pipelines")
			sessionBaseDir := "/tmp/argus"

			actives, _, err := pipeline.ScanActivePipelines(pipelinesDir)
			if err != nil {
				errBytes, _ := core.ErrorEnvelope(err.Error())
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("workflow snooze failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "no active pipeline"
				errBytes, _ := core.ErrorEnvelope(msg)
				_, _ = os.Stdout.Write(errBytes)
				_, _ = os.Stdout.WriteString("\n")
				return fmt.Errorf("workflow snooze failed: %w", core.ErrNoActivePipeline)
			}

			s, loadErr := session.LoadSession(sessionBaseDir, sessionID)
			if loadErr != nil {
				if !errors.Is(loadErr, fs.ErrNotExist) {
					return fmt.Errorf("loading session: %w", loadErr)
				}
				s = &session.Session{}
			}

			snoozed := make([]string, 0, len(actives))
			for _, active := range actives {
				session.AddSnooze(s, active.InstanceID)
				snoozed = append(snoozed, active.InstanceID)
			}

			if saveErr := session.SaveSession(sessionBaseDir, sessionID, s); saveErr != nil {
				return fmt.Errorf("saving session: %w", saveErr)
			}

			outBytes, err := core.OKEnvelope(workflowSnoozeOutput{Snoozed: snoozed})
			if err != nil {
				return fmt.Errorf("marshaling output: %w", err)
			}
			_, _ = os.Stdout.Write(outBytes)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID")
	_ = cmd.MarkFlagRequired("session")

	return cmd
}
