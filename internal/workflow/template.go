package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
)

var simplePlaceholderPattern = regexp.MustCompile(`{{-?\s*\.[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*\s*-?}}`)

type templateRuntime struct {
	env         func() map[string]string
	gitBranch   func(context.Context) string
	projectRoot func() string
}

// TemplateContext stores all prompt values available to template rendering.
type TemplateContext struct {
	Workflow TemplateWorkflowContext
	Job      TemplateJobContext
	PreJob   TemplatePreJobContext
	Git      TemplateGitContext
	Project  TemplateProjectContext
	Env      map[string]string
	Jobs     map[string]TemplateJobOutputContext
}

// TemplateWorkflowContext stores workflow-scoped values for prompt rendering.
type TemplateWorkflowContext struct {
	ID          string
	Description string
}

// TemplateJobContext stores current-job values for prompt rendering.
type TemplateJobContext struct {
	ID    string
	Index int
}

// TemplatePreJobContext stores previous-job values for prompt rendering.
type TemplatePreJobContext struct {
	ID      string
	Message string
}

// TemplateGitContext stores git-scoped values for prompt rendering.
type TemplateGitContext struct {
	Branch string
}

// TemplateProjectContext stores project-scoped values for prompt rendering.
type TemplateProjectContext struct {
	Root string
}

// TemplateJobOutputContext stores completed-job values for prompt rendering.
type TemplateJobOutputContext struct {
	Message string
}

// PipelineJobData holds per-job runtime data needed for template rendering.
// This type decouples the template engine from the pipeline package to avoid circular imports.
type PipelineJobData struct {
	StartedAt string
	EndedAt   *string
	Message   *string
}

// BuildContext assembles the template context for the selected workflow job.
func BuildContext(renderCtx context.Context, jobs map[string]*PipelineJobData, w *Workflow, jobIdx int) *TemplateContext {
	return buildContextWithRuntime(renderCtx, jobs, w, jobIdx, templateRuntime{})
}

func buildContextWithRuntime(renderCtx context.Context, jobs map[string]*PipelineJobData, w *Workflow, jobIdx int, runtime templateRuntime) *TemplateContext {
	if runtime.env == nil {
		runtime.env = buildEnvContext
	}
	if runtime.gitBranch == nil {
		runtime.gitBranch = gitBranch
	}
	if runtime.projectRoot == nil {
		runtime.projectRoot = projectRoot
	}

	templateCtx := &TemplateContext{
		Env:  runtime.env(),
		Jobs: buildJobsContext(jobs),
		Git: TemplateGitContext{
			Branch: runtime.gitBranch(renderCtx),
		},
		Project: TemplateProjectContext{
			Root: runtime.projectRoot(),
		},
	}

	if w == nil {
		return templateCtx
	}

	templateCtx.Workflow = TemplateWorkflowContext{
		ID:          w.ID,
		Description: w.Description,
	}

	if jobIdx < 0 || jobIdx >= len(w.Jobs) {
		return templateCtx
	}

	templateCtx.Job = TemplateJobContext{
		ID:    w.Jobs[jobIdx].ID,
		Index: jobIdx,
	}

	if jobIdx == 0 {
		return templateCtx
	}

	previousJobID := w.Jobs[jobIdx-1].ID
	templateCtx.PreJob = TemplatePreJobContext{
		ID:      previousJobID,
		Message: previousJobMessage(jobs, previousJobID),
	}

	return templateCtx
}

// RenderPrompt substitutes known placeholders and preserves unresolved ones.
func RenderPrompt(prompt string, ctx *TemplateContext) (string, []string) {
	if _, err := template.New("prompt").Parse(prompt); err != nil {
		return prompt, []string{fmt.Sprintf("invalid template syntax: %v", err)}
	}

	if ctx == nil {
		ctx = &TemplateContext{}
	}

	data := ctx.templateData()
	matches := simplePlaceholderPattern.FindAllStringIndex(prompt, -1)
	if len(matches) == 0 {
		return prompt, nil
	}

	var rendered strings.Builder
	warnings := make([]string, 0)
	lastIndex := 0

	for _, match := range matches {
		rendered.WriteString(prompt[lastIndex:match[0]])

		placeholder := prompt[match[0]:match[1]]
		value, warning := renderPlaceholder(placeholder, data)
		if warning != "" {
			rendered.WriteString(placeholder)
			warnings = append(warnings, warning)
		} else {
			rendered.WriteString(value)
		}

		lastIndex = match[1]
	}

	rendered.WriteString(prompt[lastIndex:])

	return rendered.String(), warnings
}

func (ctx *TemplateContext) templateData() map[string]any {
	jobs := make(map[string]map[string]string, len(ctx.Jobs))
	for jobID, job := range ctx.Jobs {
		jobs[jobID] = map[string]string{
			"message": job.Message,
		}
	}

	return map[string]any{
		"workflow": map[string]string{
			"id":          ctx.Workflow.ID,
			"description": ctx.Workflow.Description,
		},
		"job": map[string]any{
			"id":    ctx.Job.ID,
			"index": ctx.Job.Index,
		},
		"pre_job": map[string]string{
			"id":      ctx.PreJob.ID,
			"message": ctx.PreJob.Message,
		},
		"git": map[string]string{
			"branch": ctx.Git.Branch,
		},
		"project": map[string]string{
			"root": ctx.Project.Root,
		},
		"env":  ctx.Env,
		"jobs": jobs,
	}
}

func renderPlaceholder(placeholder string, data map[string]any) (string, string) {
	tmpl, err := template.New("placeholder").Option("missingkey=error").Parse(placeholder)
	if err != nil {
		return "", fmt.Sprintf("invalid template placeholder %q: %v", placeholder, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Sprintf("template placeholder %q could not be resolved: %v", placeholder, err)
	}

	return rendered.String(), ""
}

func buildEnvContext() map[string]string {
	env := os.Environ()
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		values[key] = value
	}

	return values
}

func buildJobsContext(jobData map[string]*PipelineJobData) map[string]TemplateJobOutputContext {
	jobs := make(map[string]TemplateJobOutputContext)
	for jobID, data := range jobData {
		if data == nil || data.Message == nil || *data.Message == "" || data.EndedAt == nil {
			continue
		}

		jobs[jobID] = TemplateJobOutputContext{Message: *data.Message}
	}

	return jobs
}

func previousJobMessage(jobData map[string]*PipelineJobData, previousJobID string) string {
	data := jobData[previousJobID]
	if data == nil || data.Message == nil {
		return ""
	}

	return *data.Message
}

func gitBranch(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

func projectRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return ""
	}

	return root
}
