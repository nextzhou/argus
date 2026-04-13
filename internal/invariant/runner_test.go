package invariant

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCheckWithRuntime(t *testing.T) {
	tests := []struct {
		name         string
		buildContext func(*testing.T) context.Context
		buildRuntime func(*testing.T) checkRuntime
		check        *Invariant
		assertResult func(*testing.T, *CheckResult)
	}{
		{
			name: "all steps pass",
			buildContext: func(t *testing.T) context.Context {
				t.Helper()
				t.Helper()
				return t.Context()
			},
			buildRuntime: func(t *testing.T) checkRuntime {
				t.Helper()

				var seen []string
				base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				return checkRuntime{
					now:         fakeNow(base, base.Add(1*time.Millisecond), base.Add(3*time.Millisecond), base.Add(4*time.Millisecond), base.Add(7*time.Millisecond), base.Add(8*time.Millisecond)),
					stepTimeout: stepTimeout,
					slowCheckAt: SlowCheckThreshold,
					runStep: func(_ context.Context, script string, projectRoot string) stepExecutionResult {
						seen = append(seen, script)
						assert.Equal(t, "/tmp/project", projectRoot)
						return stepExecutionResult{status: stepStatusPass}
					},
				}
			},
			check: &Invariant{
				ID: "all-pass",
				Check: []CheckStep{
					{Description: "step one", Shell: "echo 1"},
					{Description: "step two", Shell: "echo 2"},
				},
			},
			assertResult: func(t *testing.T, result *CheckResult) {
				t.Helper()
				t.Helper()
				require.NotNil(t, result)
				assert.Equal(t, "all-pass", result.InvariantID)
				assert.True(t, result.Passed)
				assert.False(t, result.SlowCheck)
				require.Len(t, result.Steps, 2)
				assertStepStatuses(t, result, []string{"pass", "pass"})
				assert.Equal(t, CheckStep{Description: "step one", Shell: "echo 1"}, result.Steps[0].Check)
				assert.Equal(t, CheckStep{Description: "step two", Shell: "echo 2"}, result.Steps[1].Check)
				assert.Equal(t, 2*time.Millisecond, result.Steps[0].Duration)
				assert.Equal(t, 3*time.Millisecond, result.Steps[1].Duration)
				assert.Equal(t, 8*time.Millisecond, result.TotalTime)
			},
		},
		{
			name: "step failure short circuits remaining steps",
			buildContext: func(t *testing.T) context.Context {
				t.Helper()
				t.Helper()
				return t.Context()
			},
			buildRuntime: func(t *testing.T) checkRuntime {
				t.Helper()
				t.Helper()

				base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				call := 0
				exitCode := 2
				return checkRuntime{
					now:         fakeNow(base, base.Add(1*time.Millisecond), base.Add(2*time.Millisecond), base.Add(3*time.Millisecond), base.Add(5*time.Millisecond), base.Add(6*time.Millisecond)),
					stepTimeout: stepTimeout,
					slowCheckAt: SlowCheckThreshold,
					runStep: func(_ context.Context, _ string, _ string) stepExecutionResult {
						call++
						if call == 1 {
							return stepExecutionResult{status: stepStatusPass}
						}
						return stepExecutionResult{
							output:      "boom",
							status:      stepStatusFail,
							exitCode:    &exitCode,
							failureKind: stepFailureExit,
						}
					},
				}
			},
			check: &Invariant{
				ID: "short-circuit",
				Check: []CheckStep{
					{Description: "pass first", Shell: ":"},
					{Description: "fail second", Shell: "exit 2"},
					{Description: "must be skipped", Shell: "echo skipped"},
				},
			},
			assertResult: func(t *testing.T, result *CheckResult) {
				t.Helper()
				t.Helper()
				require.NotNil(t, result)
				assert.False(t, result.Passed)
				require.Len(t, result.Steps, 3)
				assertStepStatuses(t, result, []string{"pass", "fail", "skip"})
				assert.Equal(t, CheckStep{Description: "fail second", Shell: "exit 2"}, result.Steps[1].Check)
				assert.Equal(t, "boom", result.Steps[1].Output)
				require.NotNil(t, result.Steps[1].ExitCode)
				assert.Equal(t, 2, *result.Steps[1].ExitCode)
				assert.Equal(t, stepFailureExit, result.Steps[1].FailureKind)
				assert.Empty(t, result.Steps[2].Output)
			},
		},
		{
			name: "caller cancellation skips remaining steps",
			buildContext: func(t *testing.T) context.Context {
				t.Helper()
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				return context.WithValue(ctx, cancelKey{}, cancel)
			},
			buildRuntime: func(t *testing.T) checkRuntime {
				t.Helper()
				t.Helper()

				base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				call := 0
				return checkRuntime{
					now:         fakeNow(base, base.Add(1*time.Millisecond), base.Add(2*time.Millisecond), base.Add(3*time.Millisecond), base.Add(4*time.Millisecond), base.Add(5*time.Millisecond)),
					stepTimeout: stepTimeout,
					slowCheckAt: SlowCheckThreshold,
					runStep: func(ctx context.Context, _ string, _ string) stepExecutionResult {
						call++
						if call == 1 {
							return stepExecutionResult{status: stepStatusPass}
						}
						cancel, ok := ctx.Value(cancelKey{}).(context.CancelFunc)
						require.True(t, ok, "context should carry cancel func")
						cancel()
						return stepExecutionResult{
							output:      buildFailureResult(ctx, "", context.Canceled, stepTimeout).output,
							status:      stepStatusFail,
							failureKind: stepFailureCanceled,
						}
					},
				}
			},
			check: &Invariant{
				ID: "canceled",
				Check: []CheckStep{
					{Description: "mark first step", Shell: "touch step-one.done"},
					{Description: "canceled while running", Shell: "sleep 10"},
					{Description: "must be skipped", Shell: ":"},
				},
			},
			assertResult: func(t *testing.T, result *CheckResult) {
				t.Helper()
				t.Helper()
				require.NotNil(t, result)
				assert.False(t, result.Passed)
				require.Len(t, result.Steps, 3)
				assertStepStatuses(t, result, []string{"pass", "fail", "skip"})
				assert.Contains(t, result.Steps[1].Output, "command canceled")
				assert.Equal(t, stepFailureCanceled, result.Steps[1].FailureKind)
			},
		},
		{
			name: "slow check is flagged when total time exceeds threshold",
			buildContext: func(t *testing.T) context.Context {
				t.Helper()
				t.Helper()
				return t.Context()
			},
			buildRuntime: func(t *testing.T) checkRuntime {
				t.Helper()
				t.Helper()

				base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				return checkRuntime{
					now:         fakeNow(base, base.Add(1*time.Second), base.Add(4*time.Second), base.Add(4*time.Second)),
					stepTimeout: stepTimeout,
					slowCheckAt: 2 * time.Second,
					runStep: func(_ context.Context, _ string, _ string) stepExecutionResult {
						return stepExecutionResult{status: stepStatusPass}
					},
				}
			},
			check: &Invariant{
				ID:    "slow",
				Check: []CheckStep{{Description: "slow but passing", Shell: ":"}},
			},
			assertResult: func(t *testing.T, result *CheckResult) {
				t.Helper()
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Passed)
				assert.True(t, result.SlowCheck)
				assert.Equal(t, 3*time.Second, result.Steps[0].Duration)
				assert.Equal(t, 4*time.Second, result.TotalTime)
			},
		},
		{
			name: "empty shell script is a no-op pass",
			buildContext: func(t *testing.T) context.Context {
				t.Helper()
				t.Helper()
				return t.Context()
			},
			buildRuntime: func(t *testing.T) checkRuntime {
				t.Helper()
				t.Helper()

				base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				return checkRuntime{
					now:         fakeNow(base, base.Add(time.Millisecond), base.Add(2*time.Millisecond), base.Add(2*time.Millisecond)),
					stepTimeout: stepTimeout,
					slowCheckAt: SlowCheckThreshold,
					runStep: func(_ context.Context, _ string, _ string) stepExecutionResult {
						return stepExecutionResult{status: stepStatusPass}
					},
				}
			},
			check: &Invariant{
				ID:    "empty-shell",
				Check: []CheckStep{{Description: "whitespace only", Shell: " \n\t "}},
			},
			assertResult: func(t *testing.T, result *CheckResult) {
				t.Helper()
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Passed)
				require.Len(t, result.Steps, 1)
				assertStepStatuses(t, result, []string{"pass"})
				assert.Empty(t, result.Steps[0].Output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.buildContext(t)
			runtime := tt.buildRuntime(t)
			result := runCheckWithRuntime(ctx, tt.check, "/tmp/project", runtime)
			tt.assertResult(t, result)
		})
	}
}

func TestRunCheckRealShell(t *testing.T) {
	t.Run("injects argus project root", func(t *testing.T) {
		projectRoot := t.TempDir()
		inv := &Invariant{
			ID: "env",
			Check: []CheckStep{{
				Description: "sees absolute project root",
				Shell:       `test "$ARGUS_PROJECT_ROOT" = '` + projectRoot + `'`,
			}},
		}

		result := RunCheck(t.Context(), inv, projectRoot)

		require.NotNil(t, result)
		assert.True(t, result.Passed)
		require.Len(t, result.Steps, 1)
		assertStepStatuses(t, result, []string{"pass"})
	})

	t.Run("caps combined stdout and stderr on failure", func(t *testing.T) {
		projectRoot := t.TempDir()
		inv := &Invariant{
			ID: "output-cap",
			Check: []CheckStep{{
				Description: "emit large output",
				Shell:       `printf 'boom\n' >&2; i=0; while [ "$i" -lt 9000 ]; do printf 'a'; i=$((i+1)); done; exit 2`,
			}},
		}

		result := RunCheck(t.Context(), inv, projectRoot)

		require.NotNil(t, result)
		assert.False(t, result.Passed)
		require.Len(t, result.Steps, 1)
		assertStepStatuses(t, result, []string{"fail"})
		assert.Contains(t, result.Steps[0].Output, "boom")
		assert.LessOrEqual(t, len(result.Steps[0].Output), outputCap)
	})

	t.Run("records exit code for ordinary shell failure", func(t *testing.T) {
		projectRoot := t.TempDir()
		inv := &Invariant{
			ID: "exit-code",
			Check: []CheckStep{{
				Description: "returns one",
				Shell:       `exit 1`,
			}},
		}

		result := RunCheck(t.Context(), inv, projectRoot)

		require.NotNil(t, result)
		assert.False(t, result.Passed)
		require.Len(t, result.Steps, 1)
		require.NotNil(t, result.Steps[0].ExitCode)
		assert.Equal(t, 1, *result.Steps[0].ExitCode)
		assert.Equal(t, stepFailureExit, result.Steps[0].FailureKind)
		assert.Empty(t, result.Steps[0].Output)
	})
}

func TestBuildFailureResultOutput(t *testing.T) {
	t.Run("timeout appends timeout diagnostic", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)

		output := buildFailureResult(ctx, "partial output", errors.New("exit status 1"), 5*time.Second).output

		assert.Contains(t, output, "partial output")
		assert.Contains(t, strings.ToLower(output), "timeout")
		assert.Contains(t, output, "5s")
	})

	t.Run("canceled appends canceled diagnostic", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		output := buildFailureResult(ctx, "", errors.New("signal: killed"), stepTimeout).output

		assert.Contains(t, output, "command canceled")
	})

	t.Run("generic exec error appends error message", func(t *testing.T) {
		output := buildFailureResult(t.Context(), "partial output", errors.New("signal: killed"), stepTimeout).output

		assert.Contains(t, output, "partial output")
		assert.Contains(t, output, "signal: killed")
	})
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
	assert.Empty(t, result.Status)
	assert.Empty(t, result.Output)
	assert.Nil(t, result.ExitCode)
	assert.Empty(t, result.FailureKind)
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

type cancelKey struct{}

func fakeNow(times ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if index >= len(times) {
			panic(fmt.Sprintf("fakeNow exhausted after %d calls", len(times)))
		}
		current := times[index]
		index++
		return current
	}
}
