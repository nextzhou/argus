package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckSetupIntegrity_SetUp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	installFakeArgusBinary(t)

	createArgusDir(t, projectRoot, "workflows")
	createArgusDir(t, projectRoot, "invariants")

	result := CheckSetupIntegrity(projectRoot)

	assert.Equal(t, "setup-integrity", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "project-level Argus setup is complete")
}

func TestCheckSetupIntegrity_MissingArgusDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	result := CheckSetupIntegrity(projectRoot)

	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, ".argus")
}

func TestCheckHookConfig_ValidHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	installFakeArgusBinary(t)

	writeClaudeHooks(t, projectRoot)
	writeCodexHooks(t, projectRoot)
	writeOpenCodePlugin(t, projectRoot)

	result := CheckHookConfig(projectRoot)

	assert.Equal(t, "hook-config", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "valid")
}

func TestCheckHookConfig_SkipMissingAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	installFakeArgusBinary(t)

	writeClaudeHooks(t, projectRoot)

	result := CheckHookConfig(projectRoot)

	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "claude-code")
	assert.NotContains(t, result.Message, "codex invalid")
}

func TestCheckWorkflowFiles_Valid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeWorkflowFile(t, projectRoot, "release.yaml", "release")

	result := CheckWorkflowFiles(projectRoot)

	assert.Equal(t, "workflow-files", result.Name)
	assert.Equal(t, "pass", result.Status)
}

func TestCheckWorkflowFiles_FileNameMustMatchID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeWorkflowFile(t, projectRoot, "wrong-name.yaml", "release")

	result := CheckWorkflowFiles(projectRoot)

	assert.Equal(t, "workflow-files", result.Name)
	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, `expected "release.yaml"`)
}

func TestCheckInvariantFiles_Valid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeWorkflowFile(t, projectRoot, "release.yaml", "release")
	writeInvariantFile(t, projectRoot, "release-check.yaml", "release-check", "release")

	result := CheckInvariantFiles(projectRoot)

	assert.Equal(t, "invariant-files", result.Name)
	assert.Equal(t, "pass", result.Status)
}

func TestCheckInvariantFiles_MisnamedWorkflowTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeWorkflowFile(t, projectRoot, "wrong-name.yaml", "release")
	writeInvariantFile(t, projectRoot, "release-check.yaml", "release-check", "release")

	result := CheckInvariantFiles(projectRoot)

	assert.Equal(t, "invariant-files", result.Name)
	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, `referenced workflow "release" not found`)
}

func TestCheckBuiltinInvariants_NoInvariants(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	createArgusDir(t, projectRoot, "invariants")

	result := CheckBuiltinInvariants(projectRoot)

	assert.Equal(t, "builtin-invariants", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "no built-in invariants")
}

func TestCheckBuiltinInvariants_SkipsMisnamedBuiltinFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	createArgusDir(t, projectRoot, "invariants")

	writeInvariantFile(t, projectRoot, "wrong-name.yaml", "argus-project-init", "argus-project-init")

	result := CheckBuiltinInvariants(projectRoot)

	assert.Equal(t, "builtin-invariants", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "no built-in invariants")
}

func TestCheckSkillIntegrity_Present(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeProjectSkills(t, projectRoot, ".agents")
	writeProjectSkills(t, projectRoot, ".claude")

	result := CheckSkillIntegrity(projectRoot)

	assert.Equal(t, "skill-integrity", result.Name)
	assert.Equal(t, "pass", result.Status)
}

func TestCheckSkillIntegrity_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	result := CheckSkillIntegrity(projectRoot)

	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, ".agents/skills/argus-doctor/SKILL.md")
	assert.Contains(t, result.Message, ".claude/skills/argus-doctor/SKILL.md")
}

func TestCheckGitignore_Complete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeRepoFile(t, projectRoot, ".gitignore", ".argus/pipelines/\n.argus/logs/\n.argus/tmp/\n")

	result := CheckGitignore(projectRoot)

	assert.Equal(t, "gitignore", result.Name)
	assert.Equal(t, "pass", result.Status)
}

func TestCheckGitignore_MissingEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeRepoFile(t, projectRoot, ".gitignore", ".argus/pipelines/\n")

	result := CheckGitignore(projectRoot)

	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, ".argus/logs/")
	assert.Contains(t, result.Message, ".argus/tmp/")
}

func TestCheckLogHealth_NoLog(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	projectRoot := t.TempDir()

	result := CheckLogHealth(projectRoot)

	assert.Equal(t, "log-health", result.Name)
	assert.Equal(t, "skip", result.Status)
	assert.Contains(t, result.Message, "no log file")
}

func TestCheckLogHealth_ErrorEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	writeRepoFile(t, projectRoot, filepath.Join(".argus", "logs", "hook.log"), "20260408T071500Z [tick] ERROR broken\n20260408T071600Z [trap] OK fine\n")

	result := CheckLogHealth(projectRoot)

	assert.Equal(t, "fail", result.Status)
	assert.Contains(t, result.Message, "1 error")
}

func TestCheckLogHealth_CleanLog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	writeRepoFile(t, projectRoot, filepath.Join(".argus", "logs", "hook.log"), "20260408T071500Z [tick] OK clean\n")

	result := CheckLogHealth(projectRoot)

	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "no errors")
}

func TestCheckVersionCompat_Compatible(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	writeWorkflowFile(t, projectRoot, "release.yaml", "release")
	writeInvariantFile(t, projectRoot, "release-check.yaml", "release-check", "release")
	writePipelineFile(t, projectRoot, "release-20260408T000000Z", "release")

	result := CheckVersionCompat(projectRoot)

	assert.Equal(t, "version-compat", result.Name)
	assert.Equal(t, "pass", result.Status)
}

func TestCheckTmpPermissions_Writable(t *testing.T) {
	result := CheckTmpPermissions()

	assert.Equal(t, "tmp-permissions", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "/tmp/argus")
}

func TestCheckPipelineData_NoPipelines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()

	result := CheckPipelineData(projectRoot)

	assert.Equal(t, "pipeline-data", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "no active pipelines")
}

func TestCheckShellEnv_Bash(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")

	result := CheckShellEnv()

	assert.Equal(t, "shell-env", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Contains(t, result.Message, "bash")
}

func TestCheckWorkspaceConfig_NoConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	result := CheckWorkspaceConfig()

	assert.Equal(t, "workspace-config", result.Name)
	assert.Equal(t, "skip", result.Status)
	assert.Contains(t, result.Message, "no workspace config")
}

func TestRunAllChecks_InstalledProject(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("SHELL", "/bin/bash")
	projectRoot := t.TempDir()
	installFakeArgusBinary(t)

	createArgusDir(t, projectRoot, "workflows")
	createArgusDir(t, projectRoot, "invariants")
	createArgusDir(t, projectRoot, "pipelines")

	writeWorkflowFile(t, projectRoot, "release.yaml", "release")
	writeWorkflowFile(t, projectRoot, "argus-project-init.yaml", "argus-project-init")
	writeInvariantFile(t, projectRoot, "release-check.yaml", "release-check", "release")
	writeInvariantFile(t, projectRoot, "argus-project-init.yaml", "argus-project-init", "argus-project-init")
	writePipelineFile(t, projectRoot, "release-20260408T000000Z", "release")

	writeProjectSkills(t, projectRoot, ".agents")
	writeProjectSkills(t, projectRoot, ".claude")
	writeRepoFile(t, projectRoot, ".gitignore", ".argus/pipelines/\n.argus/logs/\n.argus/tmp/\n")
	writeRepoFile(t, projectRoot, filepath.Join(".argus", "logs", "hook.log"), "20260408T071500Z [tick] OK pipeline ok\n")
	writeClaudeHooks(t, projectRoot)
	writeCodexHooks(t, projectRoot)
	writeOpenCodePlugin(t, projectRoot)

	workspaceDir := filepath.Join(homeDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o700))
	writeHomeFile(t, homeDir, filepath.Join(".config", "argus", "config.yaml"), "workspaces:\n  - ~/workspace\n")
	writeHomeFile(t, homeDir, filepath.Join(".claude", "settings.json"), "{}")
	writeHomeFile(t, homeDir, filepath.Join(".codex", "hooks.json"), "{}")
	writeHomeFile(t, homeDir, filepath.Join(".config", "opencode", "plugins", "argus.ts"), "export default {}\n")

	results := RunAllChecks(projectRoot)
	require.Len(t, results, 13)

	for _, result := range results {
		assert.NotEqual(t, "fail", result.Status, result.Name)
	}

	byName := mapResultsByName(results)
	assert.Equal(t, "pass", byName["workflow-files"].Status)
	assert.Equal(t, "pass", byName["invariant-files"].Status)
	assert.Equal(t, "pass", byName["builtin-invariants"].Status)
	assert.Equal(t, "pass", byName["workspace-config"].Status)
	assert.Equal(t, "pass", byName["shell-env"].Status)
	assert.Equal(t, "pass", byName["tmp-permissions"].Status)
}

func installFakeArgusBinary(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	argusPath := filepath.Join(binDir, "argus")
	writeRepoFile(t, binDir, "argus", "#!/bin/sh\nexit 0\n")
	//nolint:gosec // The fake argus binary must be executable for this test to validate PATH lookup behavior.
	require.NoError(t, os.Chmod(argusPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func createArgusDir(t *testing.T, projectRoot string, parts ...string) {
	t.Helper()

	pathParts := append([]string{projectRoot, ".argus"}, parts...)
	dir := filepath.Join(pathParts...)
	require.NoError(t, os.MkdirAll(dir, 0o700))
}

func writeWorkflowFile(t *testing.T, projectRoot string, fileName string, workflowID string) {
	t.Helper()

	content := "version: " + core.SchemaVersion + "\n" +
		"id: " + workflowID + "\n" +
		"description: test workflow\n" +
		"jobs:\n" +
		"  - id: run_check\n" +
		"    prompt: run check\n"
	writeRepoFile(t, projectRoot, filepath.Join(".argus", "workflows", fileName), content)
}

func writeInvariantFile(t *testing.T, projectRoot string, fileName string, invariantID string, workflowID string) {
	t.Helper()

	order := "20"
	if invariantID == "argus-project-init" {
		order = "10"
	}

	content := "version: " + core.SchemaVersion + "\n" +
		"id: " + invariantID + "\n" +
		"order: " + order + "\n" +
		"workflow: " + workflowID + "\n" +
		"check:\n" +
		"  - description: validate setup\n" +
		"    shell: test -d .argus\n"
	writeRepoFile(t, projectRoot, filepath.Join(".argus", "invariants", fileName), content)
}

func writePipelineFile(t *testing.T, projectRoot string, instanceID string, workflowID string) {
	t.Helper()

	currentJob := "run_check"
	message := "running"
	p := &pipeline.Pipeline{
		Version:    core.SchemaVersion,
		WorkflowID: workflowID,
		Status:     "running",
		CurrentJob: &currentJob,
		StartedAt:  "20260408T000000Z",
		Jobs: map[string]*pipeline.JobData{
			currentJob: {
				StartedAt: "20260408T000000Z",
				Message:   &message,
			},
		},
	}
	require.NoError(t, pipeline.SavePipeline(filepath.Join(projectRoot, ".argus", "pipelines"), instanceID, p))
}

func writeProjectSkills(t *testing.T, projectRoot string, baseDir string) {
	t.Helper()
	for _, skillName := range lifecycle.ProjectSkillNames() {
		writeRepoFile(t, projectRoot, filepath.Join(baseDir, "skills", skillName, "SKILL.md"), "# "+skillName+"\n")
	}
}

func writeClaudeHooks(t *testing.T, projectRoot string) {
	t.Helper()
	writeRepoFile(t, projectRoot, filepath.Join(".claude", "settings.json"), `{
	  "hooks": {
	    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "`+hookWrapperCommand("claude-code")+`"}]}]
	  }
	}`)
}

func writeCodexHooks(t *testing.T, projectRoot string) {
	t.Helper()
	writeRepoFile(t, projectRoot, filepath.Join(".codex", "hooks.json"), `{
	  "hooks": {
	    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "`+hookWrapperCommand("codex")+`"}]}]
	  }
	}`)
}

func hookWrapperCommand(agent string) string {
	return "if ! command -v argus >/dev/null 2>&1; then printf '%s\\\\n' 'Argus: Please install Argus CLI. See project README for instructions.'; exit 0; fi; exec argus tick --agent " + agent
}

func writeOpenCodePlugin(t *testing.T, projectRoot string) {
	t.Helper()
	writeRepoFile(t, projectRoot, filepath.Join(".opencode", "plugins", "argus.ts"), "export default {\n  setup() {\n    \"argus tick --agent opencode\"\n  },\n}\n")
}

func writeHomeFile(t *testing.T, homeDir string, relPath string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(homeDir, relPath)), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, relPath), []byte(content), 0o600))
}

func writeRepoFile(t *testing.T, root string, relPath string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, relPath)), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte(content), 0o600))
}

func mapResultsByName(results []CheckResult) map[string]CheckResult {
	byName := make(map[string]CheckResult, len(results))
	for _, result := range results {
		byName[result.Name] = result
	}
	return byName
}
