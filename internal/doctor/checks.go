// Package doctor provides read-only Argus diagnostic checks.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nextzhou/argus/internal/assets"
	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/lifecycle"
	"github.com/nextzhou/argus/internal/pipeline"
	"github.com/nextzhou/argus/internal/scope"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/nextzhou/argus/internal/workspace"
	"gopkg.in/yaml.v3"
)

const (
	statusPass = "pass"
	statusFail = "fail"
	statusSkip = "skip"

	checkSetupIntegrity                = "setup-integrity"
	checkHookConfig                    = "hook-config"
	checkWorkflowFiles                 = "workflow-files"
	checkInvariantFiles                = "invariant-files"
	checkBuiltinChecks                 = "builtin-invariants"
	checkAutomaticInvariantDiagnostics = "automatic-invariant-diagnostics"
	checkSkillIntegrity                = "skill-integrity"
	checkGitignore                     = "gitignore"
	checkLogHealth                     = "log-health"
	checkVersionCompat                 = "version-compat"
	checkTmpPermissions                = "tmp-permissions"
	checkPipelineData                  = "pipeline-data"
	checkShellEnv                      = "shell-env"
	checkWorkspaceConfig               = "workspace-config"

	tmpArgusDir                  = "/tmp/argus"
	workspaceConfigRelativePath  = ".config/argus/config.yaml"
	globalClaudeSettingsPath     = ".claude/settings.json"
	globalCodexHooksPath         = ".codex/hooks.json"
	globalOpenCodePluginPath     = ".config/opencode/plugins/argus.ts"
	projectClaudeSettingsPath    = ".claude/settings.json"
	projectCodexHooksPath        = ".codex/hooks.json"
	projectOpenCodePluginPath    = ".opencode/plugins/argus.ts"
	builtinInvariantCheckTimeout = 30 * time.Second
)

// RunOptions configures optional doctor diagnostics.
type RunOptions struct {
	CheckInvariants bool
}

// CheckDetail contains structured data for checks that expose richer JSON output.
type CheckDetail struct {
	AutomaticInvariantDiagnostics *AutomaticInvariantDiagnostics `json:"automatic_invariant_diagnostics,omitempty"`
}

// AutomaticInvariantDiagnostics reports timing breakdowns for automatic invariant checks.
type AutomaticInvariantDiagnostics struct {
	Enabled     bool                     `json:"enabled"`
	Risk        string                   `json:"risk,omitempty"`
	ThresholdMS int64                    `json:"threshold_ms,omitempty"`
	TotalTimeMS int64                    `json:"total_time_ms,omitempty"`
	Invariants  []InvariantTimingDetails `json:"invariants,omitempty"`
}

// InvariantTimingDetails reports one invariant's total runtime and per-step timing.
type InvariantTimingDetails struct {
	ID          string                `json:"id"`
	Auto        string                `json:"auto"`
	TotalTimeMS int64                 `json:"total_time_ms"`
	Steps       []InvariantStepTiming `json:"steps"`
}

// InvariantStepTiming reports one invariant step duration and execution status.
type InvariantStepTiming struct {
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	DurationMS  int64  `json:"duration_ms"`
}

// CheckResult reports one doctor check outcome.
type CheckResult struct {
	Name       string       `json:"name"`
	Status     string       `json:"status"`
	Message    string       `json:"message"`
	Suggestion string       `json:"suggestion,omitempty"`
	Detail     *CheckDetail `json:"detail,omitempty"`
}

// RunAllChecks executes the full doctor check suite.
func RunAllChecks(projectRoot string, currentScope scope.Scope, options RunOptions) []CheckResult {
	results := make([]CheckResult, 0, 14)
	if projectRoot == "" {
		results = append(results,
			CheckSetupIntegrity(""),
			skippedProjectCheck(checkHookConfig),
			skippedProjectCheck(checkWorkflowFiles),
			skippedProjectCheck(checkInvariantFiles),
			skippedInvariantExecutionCheck(checkBuiltinChecks),
			skippedProjectCheck(checkAutomaticInvariantDiagnostics),
			skippedProjectCheck(checkSkillIntegrity),
			skippedProjectCheck(checkGitignore),
			CheckLogHealth(""),
			skippedProjectCheck(checkVersionCompat),
			CheckTmpPermissions(),
			skippedProjectCheck(checkPipelineData),
			CheckShellEnv(),
			CheckWorkspaceConfig(),
		)
		return results
	}

	results = append(results,
		CheckSetupIntegrity(projectRoot),
		CheckHookConfig(projectRoot),
		CheckWorkflowFiles(projectRoot),
		CheckInvariantFiles(projectRoot),
		checkBuiltinDiagnostics(projectRoot, options),
		CheckAutomaticInvariantDiagnostics(currentScope, options),
		CheckSkillIntegrity(projectRoot),
		CheckGitignore(projectRoot),
		CheckLogHealth(projectRoot),
		CheckVersionCompat(projectRoot),
		CheckTmpPermissions(),
		CheckPipelineData(projectRoot),
		CheckShellEnv(),
		CheckWorkspaceConfig(),
	)

	return results
}

// CheckSetupIntegrity verifies the core project setup layout.
func CheckSetupIntegrity(projectRoot string) CheckResult {
	if projectRoot == "" {
		return failResult(checkSetupIntegrity, "project-level Argus setup is missing", "run `argus setup` in the project root")
	}

	missing := make([]string, 0, 4)
	for _, relPath := range []string{".argus", filepath.Join(".argus", "workflows"), filepath.Join(".argus", "invariants")} {
		if !isExistingDirectory(filepath.Join(projectRoot, relPath)) {
			missing = append(missing, relPath+string(filepath.Separator))
		}
	}
	if _, err := exec.LookPath("argus"); err != nil {
		missing = append(missing, "argus binary in PATH")
	}

	if len(missing) > 0 {
		slices.Sort(missing)
		return failResult(
			checkSetupIntegrity,
			fmt.Sprintf("missing setup components: %s", strings.Join(missing, ", ")),
			"re-run `argus setup` and ensure the argus binary is available in PATH",
		)
	}

	return passResult(checkSetupIntegrity, "project-level Argus setup is complete")
}

// CheckHookConfig verifies existing project hook configurations.
func CheckHookConfig(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkHookConfig)
	}
	if _, err := exec.LookPath("argus"); err != nil {
		return failResult(checkHookConfig, "argus binary not found in PATH", "ensure `argus` is installed and discoverable via PATH")
	}

	type hookConfig struct {
		agent        string
		path         string
		tickCommand  string
		contentCheck bool
	}

	configs := []hookConfig{
		{
			agent:       "claude-code",
			path:        filepath.Join(projectRoot, projectClaudeSettingsPath),
			tickCommand: "argus tick --agent claude-code",
		},
		{
			agent:       "codex",
			path:        filepath.Join(projectRoot, projectCodexHooksPath),
			tickCommand: "argus tick --agent codex",
		},
		{
			agent:        "opencode",
			path:         filepath.Join(projectRoot, projectOpenCodePluginPath),
			tickCommand:  "argus tick --agent opencode",
			contentCheck: true,
		},
	}

	validatedAgents := make([]string, 0, len(configs))
	issues := make([]string, 0)
	for _, cfg := range configs {
		if !isExistingFile(cfg.path) {
			continue
		}

		if cfg.contentCheck {
			data, err := os.ReadFile(cfg.path)
			if err != nil {
				issues = append(issues, fmt.Sprintf("%s: reading %s: %v", cfg.agent, filepath.Base(cfg.path), err))
				continue
			}
			if !strings.Contains(string(data), cfg.tickCommand) {
				issues = append(issues, fmt.Sprintf("%s: missing argus tick entry", cfg.agent))
				continue
			}
		} else {
			commands, err := readCommandFields(cfg.path)
			if err != nil {
				issues = append(issues, fmt.Sprintf("%s: %v", cfg.agent, err))
				continue
			}
			if !slices.ContainsFunc(commands, func(command string) bool {
				return strings.Contains(command, cfg.tickCommand)
			}) {
				issues = append(issues, fmt.Sprintf("%s: missing argus tick entry", cfg.agent))
				continue
			}
		}

		validatedAgents = append(validatedAgents, cfg.agent)
	}

	if len(issues) > 0 {
		slices.Sort(issues)
		return failResult(checkHookConfig, strings.Join(issues, "; "), "re-run `argus setup` to restore missing hook entries")
	}
	if len(validatedAgents) == 0 {
		return passResult(checkHookConfig, "no project-level agent hook configs found")
	}

	slices.Sort(validatedAgents)
	return passResult(checkHookConfig, fmt.Sprintf("validated hook configs: %s", strings.Join(validatedAgents, ", ")))
}

// CheckWorkflowFiles validates workflow definitions.
func CheckWorkflowFiles(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkWorkflowFiles)
	}

	allowReservedID, err := builtinWorkflowAllowReservedID()
	if err != nil {
		return failResult(checkWorkflowFiles, err.Error(), "repair the embedded built-in workflow metadata before re-running doctor")
	}

	report, err := workflow.InspectDirectory(filepath.Join(projectRoot, ".argus", "workflows"), allowReservedID)
	if err != nil {
		return failResult(checkWorkflowFiles, fmt.Sprintf("inspecting workflows: %v", err), "fix workflow directory access or restore workflow files")
	}

	issues := workflowInspectIssues(report)
	if len(issues) > 0 {
		return failResult(checkWorkflowFiles, strings.Join(issues, "; "), "fix invalid workflow files or cross-file references")
	}

	return passResult(checkWorkflowFiles, "workflow files are valid")
}

// CheckInvariantFiles validates invariant definitions.
func CheckInvariantFiles(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkInvariantFiles)
	}

	workflowDir := filepath.Join(projectRoot, ".argus", "workflows")
	allowReservedID, err := builtinInvariantAllowReservedID()
	if err != nil {
		return failResult(checkInvariantFiles, err.Error(), "repair the embedded built-in invariant metadata before re-running doctor")
	}

	report, err := invariant.InspectDirectory(filepath.Join(projectRoot, ".argus", "invariants"), func(id string) bool {
		return workflow.ExistsAtExpectedPath(workflowDir, id)
	}, allowReservedID)
	if err != nil {
		return failResult(checkInvariantFiles, fmt.Sprintf("inspecting invariants: %v", err), "fix invariant directory access or restore invariant files")
	}

	issues := invariantInspectIssues(report)
	if len(issues) > 0 {
		return failResult(checkInvariantFiles, strings.Join(issues, "; "), "fix invalid invariant files or missing workflow references")
	}

	return passResult(checkInvariantFiles, "invariant files are valid")
}

// CheckBuiltinInvariants runs built-in invariant shell checks.
func CheckBuiltinInvariants(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkBuiltinChecks)
	}

	builtinIDs, err := assets.BuiltinInvariantIDs()
	if err != nil {
		return failResult(checkBuiltinChecks, fmt.Sprintf("loading built-in invariants: %v", err), "repair the embedded built-in invariant metadata before re-running doctor")
	}

	entries, err := os.ReadDir(filepath.Join(projectRoot, ".argus", "invariants"))
	if err != nil {
		return failResult(checkBuiltinChecks, fmt.Sprintf("reading built-in invariants: %v", err), "restore the invariant directory before re-running doctor")
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(projectRoot, ".argus", "invariants", entry.Name())
		inv, parseErr := invariant.ParseInvariantFile(path)
		if parseErr != nil {
			continue
		}
		if _, ok := builtinIDs[inv.ID]; !ok {
			continue
		}
		if !core.DefinitionFileNameMatchesID(entry.Name(), inv.ID) {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return passResult(checkBuiltinChecks, "no built-in invariants found")
	}

	slices.Sort(files)
	ctx, cancel := context.WithTimeout(context.Background(), builtinInvariantCheckTimeout)
	defer cancel()

	issues := make([]string, 0)
	for _, name := range files {
		inv, parseErr := invariant.ParseInvariantFile(filepath.Join(projectRoot, ".argus", "invariants", name))
		if parseErr != nil {
			issues = append(issues, fmt.Sprintf("%s: %v", name, parseErr))
			continue
		}

		check := invariant.RunCheck(ctx, inv, projectRoot)
		if !check.Passed {
			issues = append(issues, describeInvariantFailure(inv.ID, check))
		}
	}
	if ctx.Err() != nil {
		issues = append(issues, fmt.Sprintf("built-in invariant checks timed out: %v", ctx.Err()))
	}
	if len(issues) > 0 {
		return failResult(checkBuiltinChecks, strings.Join(issues, "; "), "inspect the failing built-in invariant output and repair the underlying project state")
	}

	return passResult(checkBuiltinChecks, "all built-in invariants passed")
}

func checkBuiltinDiagnostics(projectRoot string, options RunOptions) CheckResult {
	if !options.CheckInvariants {
		return skippedInvariantExecutionCheck(checkBuiltinChecks)
	}

	return CheckBuiltinInvariants(projectRoot)
}

// CheckAutomaticInvariantDiagnostics profiles automatic invariant execution.
func CheckAutomaticInvariantDiagnostics(currentScope scope.Scope, options RunOptions) CheckResult {
	if !options.CheckInvariants {
		return CheckResult{
			Name:       checkAutomaticInvariantDiagnostics,
			Status:     statusSkip,
			Message:    "automatic invariant deep diagnostics are disabled by default because they execute project-defined shell checks",
			Suggestion: "use the `argus-doctor` skill to assess invariant risk, then re-run `argus doctor --check-invariants` if safe",
			Detail: &CheckDetail{
				AutomaticInvariantDiagnostics: &AutomaticInvariantDiagnostics{
					Enabled: false,
					Risk:    "executes project-defined automatic invariant shell checks",
				},
			},
		}
	}

	if currentScope == nil {
		return skipResult(checkAutomaticInvariantDiagnostics, "current Argus scope not found")
	}

	catalog, err := currentScope.LoadInvariantCatalog()
	if err != nil {
		return failResult(
			checkAutomaticInvariantDiagnostics,
			fmt.Sprintf("loading invariant catalog: %v", err),
			"repair the invariant catalog before re-running `argus doctor --check-invariants`",
		)
	}

	detail := &AutomaticInvariantDiagnostics{
		Enabled:     true,
		Risk:        "executes project-defined automatic invariant shell checks",
		ThresholdMS: invariant.SlowCheckThreshold.Milliseconds(),
		Invariants:  []InvariantTimingDetails{},
	}

	if catalog == nil || len(catalog.Invariants) == 0 {
		return passResultWithDetail(
			checkAutomaticInvariantDiagnostics,
			"no automatic invariants found for deep diagnostics",
			&CheckDetail{AutomaticInvariantDiagnostics: detail},
		)
	}

	projectRoot := currentScope.ProjectRoot()
	var total time.Duration
	for _, inv := range catalog.Invariants {
		if inv.Auto == "never" {
			continue
		}

		result := invariant.RunCheck(context.Background(), inv, projectRoot)
		total += result.TotalTime

		steps := make([]InvariantStepTiming, 0, len(result.Steps))
		for _, step := range result.Steps {
			steps = append(steps, InvariantStepTiming{
				Description: step.Description,
				Status:      step.Status,
				DurationMS:  step.Duration.Milliseconds(),
			})
		}

		detail.Invariants = append(detail.Invariants, InvariantTimingDetails{
			ID:          inv.ID,
			Auto:        inv.Auto,
			TotalTimeMS: result.TotalTime.Milliseconds(),
			Steps:       steps,
		})
	}

	detail.TotalTimeMS = total.Milliseconds()
	slices.SortFunc(detail.Invariants, func(a, b InvariantTimingDetails) int {
		switch {
		case a.TotalTimeMS > b.TotalTimeMS:
			return -1
		case a.TotalTimeMS < b.TotalTimeMS:
			return 1
		default:
			return strings.Compare(a.ID, b.ID)
		}
	})

	if len(detail.Invariants) == 0 {
		return passResultWithDetail(
			checkAutomaticInvariantDiagnostics,
			"no automatic invariants found for deep diagnostics",
			&CheckDetail{AutomaticInvariantDiagnostics: detail},
		)
	}

	message := fmt.Sprintf(
		"automatic invariant checks took %.1fs total across %d invariants",
		total.Seconds(),
		len(detail.Invariants),
	)
	if total > invariant.SlowCheckThreshold {
		return failResultWithDetail(
			checkAutomaticInvariantDiagnostics,
			message,
			"optimize the slowest automatic invariants or narrow their auto policy",
			&CheckDetail{AutomaticInvariantDiagnostics: detail},
		)
	}

	return passResultWithDetail(checkAutomaticInvariantDiagnostics, message, &CheckDetail{AutomaticInvariantDiagnostics: detail})
}

// CheckSkillIntegrity verifies managed project skill mirrors.
func CheckSkillIntegrity(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkSkillIntegrity)
	}

	missing := make([]string, 0, len(lifecycle.SkillPaths())*len(lifecycle.ProjectSkillNames()))
	for _, skillDir := range lifecycle.SkillPaths() {
		for _, skillName := range lifecycle.ProjectSkillNames() {
			skillPath := filepath.Join(projectRoot, skillDir, skillName, "SKILL.md")
			if isExistingFile(skillPath) {
				continue
			}
			missing = append(missing, filepath.Join(skillDir, skillName, "SKILL.md"))
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return failResult(checkSkillIntegrity, fmt.Sprintf("missing Argus skills under: %s", strings.Join(missing, ", ")), "re-run `argus setup` to restore project skill files")
	}

	return passResult(checkSkillIntegrity, "Argus project skill files are present in both managed directories")
}

// CheckGitignore verifies required local-only ignore rules.
func CheckGitignore(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkGitignore)
	}

	data, err := readProjectFile(projectRoot, ".gitignore")
	if err != nil {
		return failResult(checkGitignore, fmt.Sprintf("reading .gitignore: %v", err), "add the required Argus local-only paths to .gitignore")
	}

	missing := make([]string, 0, 3)
	content := string(data)
	for _, entry := range []string{".argus/pipelines/", ".argus/logs/", ".argus/tmp/"} {
		if !strings.Contains(content, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) > 0 {
		return failResult(checkGitignore, fmt.Sprintf("missing .gitignore entries: %s", strings.Join(missing, ", ")), "add the missing Argus local-only directories to .gitignore")
	}

	return passResult(checkGitignore, ".gitignore contains all required Argus local-only entries")
}

// CheckLogHealth inspects hook logs for recorded errors.
func CheckLogHealth(projectRoot string) CheckResult {
	logPath, data, err := readDoctorLog(projectRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return skipResult(checkLogHealth, "no log file found")
		}
		return failResult(checkLogHealth, fmt.Sprintf("reading hook log: %v", err), "verify log directory permissions and retry the diagnostic")
	}

	errorCount := 0
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.Contains(line, "] ERROR ") {
			errorCount++
		}
	}
	if errorCount > 0 {
		return failResult(checkLogHealth, fmt.Sprintf("hook log contains %d error entries (%s)", errorCount, logPath), "inspect the hook log and address the recorded failures")
	}

	return passResult(checkLogHealth, fmt.Sprintf("hook log contains no errors (%s)", logPath))
}

// CheckVersionCompat verifies schema compatibility across Argus files.
func CheckVersionCompat(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkVersionCompat)
	}

	files, err := collectVersionedFiles(projectRoot)
	if err != nil {
		return failResult(checkVersionCompat, fmt.Sprintf("collecting versioned files: %v", err), "restore the expected Argus directories and retry doctor")
	}

	incompatible := make([]string, 0)
	for _, file := range files {
		version, readErr := readVersionField(file)
		if readErr != nil {
			incompatible = append(incompatible, fmt.Sprintf("%s: %v", relativeToProject(projectRoot, file), readErr))
			continue
		}
		if compatErr := core.CheckCompatibility(version); compatErr != nil {
			incompatible = append(incompatible, fmt.Sprintf("%s: %v", relativeToProject(projectRoot, file), compatErr))
		}
	}
	if len(incompatible) > 0 {
		return failResult(checkVersionCompat, strings.Join(incompatible, "; "), "regenerate incompatible Argus files with the current schema version")
	}

	return passResult(checkVersionCompat, "all versioned Argus files are schema-compatible")
}

// CheckTmpPermissions verifies /tmp/argus can be written.
func CheckTmpPermissions() CheckResult {
	if err := os.MkdirAll(tmpArgusDir, 0o700); err != nil {
		return failResult(checkTmpPermissions, fmt.Sprintf("creating %s: %v", tmpArgusDir, err), "fix the temporary directory permissions for /tmp/argus")
	}

	f, err := os.CreateTemp(tmpArgusDir, "doctor-*.tmp")
	if err != nil {
		return failResult(checkTmpPermissions, fmt.Sprintf("creating temp file in %s: %v", tmpArgusDir, err), "fix the temporary directory permissions for /tmp/argus")
	}
	path := f.Name()
	if _, writeErr := f.WriteString("argus doctor\n"); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return failResult(checkTmpPermissions, fmt.Sprintf("writing temp file in %s: %v", tmpArgusDir, writeErr), "fix the temporary directory permissions for /tmp/argus")
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(path)
		return failResult(checkTmpPermissions, fmt.Sprintf("closing temp file in %s: %v", tmpArgusDir, closeErr), "fix the temporary directory permissions for /tmp/argus")
	}
	if removeErr := os.Remove(path); removeErr != nil {
		return failResult(checkTmpPermissions, fmt.Sprintf("cleaning temp file in %s: %v", tmpArgusDir, removeErr), "verify that temporary files under /tmp/argus can be removed")
	}

	return passResult(checkTmpPermissions, fmt.Sprintf("temporary directory %s is writable", tmpArgusDir))
}

// CheckPipelineData validates active pipeline files and references.
func CheckPipelineData(projectRoot string) CheckResult {
	if projectRoot == "" {
		return skippedProjectCheck(checkPipelineData)
	}

	pipelinesDir := filepath.Join(projectRoot, ".argus", "pipelines")
	actives, warnings, err := pipeline.ScanActivePipelines(pipelinesDir)
	if err != nil {
		return failResult(checkPipelineData, fmt.Sprintf("scanning pipelines: %v", err), "repair the pipeline directory and retry doctor")
	}

	issues := make([]string, 0, len(warnings)+len(actives))
	for _, warning := range warnings {
		issues = append(issues, fmt.Sprintf("%s: %v", warning.InstanceID, warning.Err))
	}
	for _, active := range actives {
		if active.Pipeline == nil {
			issues = append(issues, fmt.Sprintf("%s: missing pipeline data", active.InstanceID))
			continue
		}
		workflowPath := filepath.Join(projectRoot, ".argus", "workflows", active.Pipeline.WorkflowID+".yaml")
		if !isExistingFile(workflowPath) {
			issues = append(issues, fmt.Sprintf("%s references missing workflow %q", active.InstanceID, active.Pipeline.WorkflowID))
		}
	}
	if len(issues) > 0 {
		slices.Sort(issues)
		return failResult(checkPipelineData, strings.Join(issues, "; "), "remove corrupt pipeline files or restore the missing workflow definitions")
	}
	if len(actives) == 0 {
		return passResult(checkPipelineData, "no active pipelines found")
	}

	return passResult(checkPipelineData, fmt.Sprintf("validated %d active pipelines", len(actives)))
}

// CheckShellEnv reports whether the default shell is bash.
func CheckShellEnv() CheckResult {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "bash") {
		return passResult(checkShellEnv, fmt.Sprintf("default shell is bash (%s)", shell))
	}

	return CheckResult{
		Name:       checkShellEnv,
		Status:     statusPass,
		Message:    "default shell is not bash; invariant checks use bash",
		Suggestion: "ensure tools and environment variables needed by invariant checks are available in bash",
	}
}

// CheckWorkspaceConfig validates workspace registrations and global hooks.
func CheckWorkspaceConfig() CheckResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return failResult(checkWorkspaceConfig, fmt.Sprintf("getting home directory: %v", err), "ensure HOME is set before running doctor")
	}

	configPath := filepath.Join(homeDir, workspaceConfigRelativePath)
	if !isExistingFile(configPath) {
		return skipResult(checkWorkspaceConfig, "no workspace config found")
	}

	config, err := workspace.LoadConfig(configPath)
	if err != nil {
		return failResult(checkWorkspaceConfig, fmt.Sprintf("loading workspace config: %v", err), "repair ~/.config/argus/config.yaml and retry doctor")
	}

	issues := make([]string, 0, len(config.Workspaces)+3)
	for _, registered := range config.Workspaces {
		expanded := workspace.ExpandPath(registered)
		info, statErr := os.Stat(expanded)
		if statErr != nil {
			issues = append(issues, fmt.Sprintf("workspace %q: %v", registered, statErr))
			continue
		}
		if !info.IsDir() {
			issues = append(issues, fmt.Sprintf("workspace %q is not a directory", registered))
		}
	}

	for _, relPath := range []string{globalClaudeSettingsPath, globalCodexHooksPath, globalOpenCodePluginPath} {
		fullPath := filepath.Join(homeDir, relPath)
		if !isExistingFile(fullPath) {
			issues = append(issues, fmt.Sprintf("missing global hook config %s", fullPath))
		}
	}

	if len(issues) > 0 {
		slices.Sort(issues)
		return failResult(checkWorkspaceConfig, strings.Join(issues, "; "), "repair workspace registrations or re-run `argus setup --workspace <path>`")
	}

	return passResult(checkWorkspaceConfig, fmt.Sprintf("workspace config is valid for %d workspaces", len(config.Workspaces)))
}

func passResult(name string, message string) CheckResult {
	return CheckResult{Name: name, Status: statusPass, Message: message}
}

func failResult(name string, message string, suggestion string) CheckResult {
	return CheckResult{Name: name, Status: statusFail, Message: message, Suggestion: suggestion}
}

func passResultWithDetail(name string, message string, detail *CheckDetail) CheckResult {
	result := passResult(name, message)
	result.Detail = detail
	return result
}

func failResultWithDetail(name string, message string, suggestion string, detail *CheckDetail) CheckResult {
	result := failResult(name, message, suggestion)
	result.Detail = detail
	return result
}

func readProjectFile(projectRoot string, relativePath string) ([]byte, error) {
	path := filepath.Join(projectRoot, relativePath)
	if err := core.ValidatePath(projectRoot, path); err != nil {
		return nil, fmt.Errorf("validating %s path: %w", relativePath, err)
	}

	//nolint:gosec // The file path is constrained to the resolved project root via ValidatePath.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func skipResult(name string, message string) CheckResult {
	return CheckResult{Name: name, Status: statusSkip, Message: message}
}

func skippedProjectCheck(name string) CheckResult {
	return skipResult(name, "project root not found")
}

func skippedInvariantExecutionCheck(name string) CheckResult {
	return CheckResult{
		Name:       name,
		Status:     statusSkip,
		Message:    "invariant shell checks are disabled by default because they execute shell commands",
		Suggestion: "use the `argus-doctor` skill to assess invariant risk, then re-run `argus doctor --check-invariants` if safe",
	}
}

func isExistingDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readCommandFields(path string) ([]string, error) {
	//nolint:gosec // The caller resolves concrete config file paths before readCommandFields parses them.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}

	var parsed any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}

	commands := make([]string, 0)
	collectCommandFields(parsed, &commands)
	return commands, nil
}

func collectCommandFields(node any, commands *[]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, nested := range value {
			if key == "command" {
				if command, ok := nested.(string); ok {
					*commands = append(*commands, command)
				}
				continue
			}
			collectCommandFields(nested, commands)
		}
	case []any:
		for _, nested := range value {
			collectCommandFields(nested, commands)
		}
	}
}

func workflowInspectIssues(report *workflow.InspectReport) []string {
	if report == nil {
		return []string{"workflow inspection returned no report"}
	}

	fileNames := make([]string, 0, len(report.Files))
	for name := range report.Files {
		fileNames = append(fileNames, name)
	}
	slices.Sort(fileNames)

	issues := make([]string, 0)
	for _, name := range fileNames {
		fileResult := report.Files[name]
		if fileResult == nil {
			issues = append(issues, fmt.Sprintf("%s: missing inspection result", name))
			continue
		}
		for _, fieldErr := range fileResult.Errors {
			issues = append(issues, formatFieldError(name, fieldErr.Path, fieldErr.Message))
		}
	}

	return issues
}

func invariantInspectIssues(report *invariant.InspectReport) []string {
	if report == nil {
		return []string{"invariant inspection returned no report"}
	}

	fileNames := make([]string, 0, len(report.Files))
	for name := range report.Files {
		fileNames = append(fileNames, name)
	}
	slices.Sort(fileNames)

	issues := make([]string, 0)
	for _, name := range fileNames {
		fileResult := report.Files[name]
		if fileResult == nil {
			issues = append(issues, fmt.Sprintf("%s: missing inspection result", name))
			continue
		}
		for _, fieldErr := range fileResult.Errors {
			issues = append(issues, formatFieldError(name, fieldErr.Path, fieldErr.Message))
		}
	}

	return issues
}

func formatFieldError(fileName string, fieldPath string, message string) string {
	if fieldPath == "" {
		return fmt.Sprintf("%s: %s", fileName, message)
	}
	return fmt.Sprintf("%s:%s %s", fileName, fieldPath, message)
}

func builtinWorkflowAllowReservedID() (func(string) bool, error) {
	ids, err := assets.BuiltinWorkflowIDs()
	if err != nil {
		return nil, fmt.Errorf("loading built-in workflows: %w", err)
	}
	return allowReservedIDs(ids), nil
}

func builtinInvariantAllowReservedID() (func(string) bool, error) {
	ids, err := assets.BuiltinInvariantIDs()
	if err != nil {
		return nil, fmt.Errorf("loading built-in invariants: %w", err)
	}
	return allowReservedIDs(ids), nil
}

func allowReservedIDs(ids map[string]struct{}) func(string) bool {
	return func(id string) bool {
		_, ok := ids[id]
		return ok
	}
}

func describeInvariantFailure(invariantID string, check *invariant.CheckResult) string {
	if check == nil {
		return fmt.Sprintf("%s: invariant run returned no result", invariantID)
	}
	for _, step := range check.Steps {
		if step.Status != statusFail {
			continue
		}
		output := strings.TrimSpace(step.Output)
		if output == "" {
			return fmt.Sprintf("%s: step %q failed", invariantID, step.Description)
		}
		return fmt.Sprintf("%s: step %q failed: %s", invariantID, step.Description, output)
	}
	return fmt.Sprintf("%s: invariant check failed", invariantID)
}

func readDoctorLog(projectRoot string) (string, []byte, error) {
	paths := make([]string, 0, 2)
	if projectRoot != "" {
		paths = append(paths, filepath.Join(projectRoot, ".argus", "logs", "hook.log"))
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("getting home directory: %w", err)
	}
	paths = append(paths, filepath.Join(homeDir, ".config", "argus", "logs", "hook.log"))

	for _, path := range paths {
		//nolint:gosec // readDoctorLog only inspects known Argus-managed log paths.
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			return path, data, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", nil, readErr
		}
	}

	return "", nil, os.ErrNotExist
}

func collectVersionedFiles(projectRoot string) ([]string, error) {
	patterns := []string{
		filepath.Join(projectRoot, ".argus", "workflows", "*.yaml"),
		filepath.Join(projectRoot, ".argus", "invariants", "*.yaml"),
		filepath.Join(projectRoot, ".argus", "pipelines", "*.yaml"),
	}

	files := make([]string, 0)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
		for _, match := range matches {
			if filepath.Base(match) == workflowSharedFileName() {
				continue
			}
			files = append(files, match)
		}
	}

	slices.Sort(files)
	return files, nil
}

func workflowSharedFileName() string {
	return "_shared.yaml"
}

func readVersionField(path string) (string, error) {
	//nolint:gosec // collectVersionedFiles enumerates concrete Argus-managed files before readVersionField reads them.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading versioned file: %w", err)
	}

	var versioned struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &versioned); err != nil {
		return "", fmt.Errorf("parsing YAML: %w", err)
	}

	return versioned.Version, nil
}

func relativeToProject(projectRoot string, path string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return path
	}
	return rel
}
