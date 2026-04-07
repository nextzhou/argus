package invariant

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCheck(t *testing.T) {
	tests := []struct {
		name         string
		buildContext func(*testing.T, string) context.Context
		buildCheck   func(*testing.T, string) *Invariant
		assertResult func(*testing.T, *CheckResult, time.Duration)
	}{
		{
			name: "all steps pass",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, _ string) *Invariant {
				t.Helper()
				return &Invariant{
					ID: "all-pass",
					Check: []CheckStep{
						{Description: "project root exists", Shell: `test -d "$ARGUS_PROJECT_ROOT"`},
						{Description: "readme absence allowed", Shell: `:`},
					},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, _ time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.Equal(t, "all-pass", result.InvariantID)
				assert.True(t, result.Passed)
				assert.False(t, result.SlowCheck)
				require.Len(t, result.Steps, 2)
				assertStepStatuses(t, result, []string{"pass", "pass"})
				for _, step := range result.Steps {
					assert.Empty(t, step.Output)
					assert.GreaterOrEqual(t, step.Duration, time.Duration(0))
				}
				assert.GreaterOrEqual(t, result.TotalTime, time.Duration(0))
			},
		},
		{
			name: "step fails and short circuits remaining steps",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, _ string) *Invariant {
				t.Helper()
				return &Invariant{
					ID: "short-circuit",
					Check: []CheckStep{
						{Description: "pass first", Shell: `:`},
						{Description: "fail second", Shell: `printf 'boom\n' >&2; i=0; while [ "$i" -lt 9000 ]; do printf 'a'; i=$((i+1)); done; exit 2`},
						{Description: "must be skipped", Shell: `exit 0`},
					},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, _ time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.False(t, result.Passed)
				require.Len(t, result.Steps, 3)
				assertStepStatuses(t, result, []string{"pass", "fail", "skip"})
				assert.Empty(t, result.Steps[0].Output)
				assert.NotEmpty(t, result.Steps[1].Output)
				assert.Contains(t, result.Steps[1].Output, "boom")
				assert.LessOrEqual(t, len(result.Steps[1].Output), 8192)
				assert.Empty(t, result.Steps[2].Output)
			},
		},
		{
			name: "step timeout fails after five seconds",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, _ string) *Invariant {
				t.Helper()
				return &Invariant{
					ID: "timeout",
					Check: []CheckStep{
						{Description: "times out", Shell: `sleep 10`},
						{Description: "must be skipped", Shell: `:`},
					},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, elapsed time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.False(t, result.Passed)
				require.Len(t, result.Steps, 2)
				assertStepStatuses(t, result, []string{"fail", "skip"})
				assert.Contains(t, strings.ToLower(result.Steps[0].Output), "timeout")
				assert.GreaterOrEqual(t, elapsed, 5*time.Second)
				assert.Less(t, elapsed, 7*time.Second)
			},
		},
		{
			name: "injects argus project root",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, projectRoot string) *Invariant {
				t.Helper()
				return &Invariant{
					ID: "env",
					Check: []CheckStep{{
						Description: "sees absolute project root",
						Shell:       `test "$ARGUS_PROJECT_ROOT" = '` + projectRoot + `'`,
					}},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, _ time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Passed)
				require.Len(t, result.Steps, 1)
				assertStepStatuses(t, result, []string{"pass"})
				assert.Empty(t, result.Steps[0].Output)
			},
		},
		{
			name: "slow check is flagged when total time exceeds two seconds",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, _ string) *Invariant {
				t.Helper()
				return &Invariant{
					ID:    "slow",
					Check: []CheckStep{{Description: "sleep long enough", Shell: `sleep 2.1`}},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, _ time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Passed)
				assert.True(t, result.SlowCheck)
				assert.Greater(t, result.TotalTime, 2*time.Second)
				require.Len(t, result.Steps, 1)
				assertStepStatuses(t, result, []string{"pass"})
			},
		},
		{
			name: "empty shell script is a no op pass",
			buildContext: func(t *testing.T, _ string) context.Context {
				t.Helper()
				return t.Context()
			},
			buildCheck: func(t *testing.T, _ string) *Invariant {
				t.Helper()
				return &Invariant{
					ID:    "empty-shell",
					Check: []CheckStep{{Description: "whitespace only", Shell: " \n\t "}},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, _ time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Passed)
				require.Len(t, result.Steps, 1)
				assertStepStatuses(t, result, []string{"pass"})
				assert.Empty(t, result.Steps[0].Output)
			},
		},
		{
			name: "caller cancellation skips remaining steps",
			buildContext: func(t *testing.T, projectRoot string) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				signalFile := filepath.Join(projectRoot, "step-one.done")
				go func() {
					for {
						if ctx.Err() != nil {
							return
						}
						if _, err := os.Stat(signalFile); err == nil {
							cancel()
							return
						}
						time.Sleep(10 * time.Millisecond)
					}
				}()
				return ctx
			},
			buildCheck: func(t *testing.T, projectRoot string) *Invariant {
				t.Helper()
				signalFile := filepath.Join(projectRoot, "step-one.done")
				return &Invariant{
					ID: "canceled",
					Check: []CheckStep{
						{Description: "mark first step", Shell: `touch "` + signalFile + `"`},
						{Description: "canceled while running", Shell: `sleep 10`},
						{Description: "must be skipped", Shell: `:`},
					},
				}
			},
			assertResult: func(t *testing.T, result *CheckResult, elapsed time.Duration) {
				t.Helper()
				require.NotNil(t, result)
				assert.False(t, result.Passed)
				require.Len(t, result.Steps, 3)
				assert.Equal(t, "pass", result.Steps[0].Status)
				assert.Equal(t, "skip", result.Steps[2].Status)
				assert.Contains(t, []string{"fail", "skip"}, result.Steps[1].Status)
				if result.Steps[1].Status == "fail" {
					assert.Contains(t, strings.ToLower(result.Steps[1].Output), "context canceled")
				}
				assert.Less(t, elapsed, 5*time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			ctx := tt.buildContext(t, projectRoot)
			inv := tt.buildCheck(t, projectRoot)

			startedAt := time.Now()
			result := RunCheck(ctx, inv, projectRoot)
			elapsed := time.Since(startedAt)

			tt.assertResult(t, result, elapsed)
		})
	}
}

func TestCheckResultZeroValue(t *testing.T) {
	var result CheckResult
	assert.Empty(t, result.InvariantID)
	assert.False(t, result.Passed)
	assert.Nil(t, result.Steps)
	assert.Zero(t, result.TotalTime)
	assert.False(t, result.SlowCheck)
}

func TestStepResultZeroValue(t *testing.T) {
	var result StepResult
	assert.Empty(t, result.Description)
	assert.Empty(t, result.Status)
	assert.Empty(t, result.Output)
	assert.Zero(t, result.Duration)
}

func assertStepStatuses(t *testing.T, result *CheckResult, want []string) {
	t.Helper()
	got := make([]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		got = append(got, step.Status)
	}
	require.True(t, slices.Equal(got, want), "step statuses mismatch: got=%v want=%v", got, want)
}
