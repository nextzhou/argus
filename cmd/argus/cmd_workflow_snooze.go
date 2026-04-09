package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "snooze",
		Short: "Snooze active pipelines for a session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pipelinesDir := filepath.Join(".argus", "pipelines")
			sessionBaseDir := "/tmp/argus"

			actives, _, err := pipeline.ScanActivePipelines(pipelinesDir)
			if err != nil {
				writeCommandError(cmd, jsonFlag, err.Error())
				return fmt.Errorf("workflow snooze failed: %w", err)
			}

			if len(actives) == 0 {
				msg := "no active pipeline"
				writeCommandError(cmd, jsonFlag, msg)
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

			out := workflowSnoozeOutput{Snoozed: snoozed}
			if jsonFlag {
				return writeJSONOK(cmd, out)
			}

			renderWorkflowSnoozeText(cmd.OutOrStdout(), snoozed)
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID")
	_ = cmd.MarkFlagRequired("session")
	bindJSONFlag(cmd, &jsonFlag)

	return cmd
}

func renderWorkflowSnoozeText(w io.Writer, snoozed []string) {
	_, _ = fmt.Fprintf(w, "Argus: Snoozed %d pipeline(s) for this session.\n", len(snoozed))
	for _, instanceID := range snoozed {
		_, _ = fmt.Fprintf(w, "- %s\n", instanceID)
	}
}
