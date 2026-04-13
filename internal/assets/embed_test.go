package assets

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"text/template"

	"github.com/nextzhou/argus/internal/core"
	"github.com/nextzhou/argus/internal/invariant"
	"github.com/nextzhou/argus/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAsset(t *testing.T) {
	data, err := ReadAsset("skills/argus-setup/SKILL.md")
	require.NoError(t, err)
	assert.Contains(t, string(data), "argus-setup")
}

func TestReadAssetNotFound(t *testing.T) {
	_, err := ReadAsset("nonexistent/file.txt")
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading asset")
}

func TestListSkills(t *testing.T) {
	names, err := ListAssets("skills")
	require.NoError(t, err)
	assert.Len(t, names, 7)

	expected := []string{
		"argus-configure-invariant",
		"argus-configure-workflow",
		"argus-doctor",
		"argus-intro",
		"argus-runtime",
		"argus-setup",
		"argus-teardown",
	}
	assert.Equal(t, expected, names)
}

func TestSkillFrontmatter(t *testing.T) {
	names, err := ListAssets("skills")
	require.NoError(t, err)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, core.ValidateSkillName(name))
			assert.True(t, core.IsArgusReserved(name))

			data, err := ReadAsset("skills/" + name + "/SKILL.md")
			require.NoError(t, err)

			content := string(data)
			assert.True(t, strings.HasPrefix(content, "---\n"), "must start with frontmatter")
			assert.Contains(t, content, "name: "+name)
			assert.Contains(t, content, "description:")
			assert.Contains(t, content, "version:")
		})
	}
}

func TestListAssets(t *testing.T) {
	names, err := ListAssets("workflows")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(names), 1)
	assert.Contains(t, names, "argus-project-init.yaml")
}

func TestWalkAssets(t *testing.T) {
	var count int
	err := WalkAssets("skills", func(_ string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 16, count)
}

func TestBuiltinWorkflow(t *testing.T) {
	data, err := ReadAsset("workflows/argus-project-init.yaml")
	require.NoError(t, err)

	w, err := workflow.ParseWorkflow(bytes.NewReader(data))
	require.NoError(t, err)

	assert.Equal(t, "argus-project-init", w.ID)
	assert.NotEmpty(t, w.Jobs)
}

func TestBuiltinInvariant(t *testing.T) {
	data, err := ReadAsset("invariants/argus-project-init.yaml")
	require.NoError(t, err)

	inv, err := invariant.ParseInvariant(bytes.NewReader(data))
	require.NoError(t, err)

	assert.Equal(t, "argus-project-init", inv.ID)
	assert.Equal(t, "argus-project-init", inv.Workflow)
	assert.NotEmpty(t, inv.Check)
	assert.NotEmpty(t, inv.Prompt)
}

func TestBuiltinInvariantProjectInit(t *testing.T) {
	data, err := ReadAsset("invariants/argus-project-setup.yaml")
	require.NoError(t, err)

	inv, err := invariant.ParseInvariant(bytes.NewReader(data))
	require.NoError(t, err)

	assert.Equal(t, "argus-project-setup", inv.ID)
}

func TestBuiltinWorkflowIDs(t *testing.T) {
	ids, err := BuiltinWorkflowIDs()
	require.NoError(t, err)
	assert.Contains(t, ids, "argus-project-init")
}

func TestBuiltinInvariantIDs(t *testing.T) {
	ids, err := BuiltinInvariantIDs()
	require.NoError(t, err)
	assert.Contains(t, ids, "argus-project-init")
	assert.Contains(t, ids, "argus-project-setup")
}

func TestPromptTemplates(t *testing.T) {
	templates := []string{
		"prompts/tick-no-pipeline.md.tmpl",
		"prompts/tick-full-context.md.tmpl",
		"prompts/tick-minimal.md.tmpl",
		"prompts/tick-snoozed.md.tmpl",
		"prompts/tick-invariant-failed.md.tmpl",
		"prompts/tick-active-pipeline-issue.md.tmpl",
		"prompts/tick-multiple-active-pipelines.md.tmpl",
	}

	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			data, err := ReadAsset(name)
			require.NoError(t, err)

			_, err = template.New(name).Parse(string(data))
			require.NoError(t, err, "template must parse without error")
		})
	}
}

func TestPromptTemplatesRender(t *testing.T) {
	// Test tick-no-pipeline renders with sample data
	data, err := ReadAsset("prompts/tick-no-pipeline.md.tmpl")
	require.NoError(t, err)

	tmpl, err := template.New("test").Parse(string(data))
	require.NoError(t, err)

	type WF struct {
		ID          string
		Description string
	}
	var buf strings.Builder
	err = tmpl.Execute(&buf, map[string]any{
		"Workflows": []WF{
			{ID: "release", Description: "Release workflow"},
		},
	})
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Argus:")
	assert.Contains(t, output, "Available workflows")
	assert.Contains(t, output, "release: Release workflow")
}

func TestPromptTemplatesEmptyData(t *testing.T) {
	templates := []struct {
		name string
		data any
	}{
		{"prompts/tick-no-pipeline.md.tmpl", map[string]any{"Workflows": nil}},
		{"prompts/tick-full-context.md.tmpl", map[string]any{
			"PipelineID": "", "WorkflowID": "", "Progress": "",
			"JobID": "", "Prompt": "", "Skill": "", "SessionID": "",
		}},
		{"prompts/tick-minimal.md.tmpl", map[string]any{
			"WorkflowID": "", "JobID": "", "Progress": "",
		}},
		{"prompts/tick-snoozed.md.tmpl", map[string]any{"Workflows": nil}},
		{"prompts/tick-invariant-failed.md.tmpl", map[string]any{
			"Failure": map[string]any{
				"ID": "", "Description": "", "Prompt": "", "WorkflowID": "", "Facts": nil,
			},
		}},
		{"prompts/tick-active-pipeline-issue.md.tmpl", map[string]any{
			"Issue": map[string]any{
				"PipelineID": "", "WorkflowID": "", "Issue": "", "InvestigateCommand": "",
				"InvestigateGuidance": "", "SessionID": "",
			},
		}},
		{"prompts/tick-multiple-active-pipelines.md.tmpl", map[string]any{
			"InstanceIDs": []string{}, "SessionID": "",
		}},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ReadAsset(tt.name)
			require.NoError(t, err)

			tmpl, err := template.New(tt.name).Parse(string(data))
			require.NoError(t, err)

			var buf strings.Builder
			err = tmpl.Execute(&buf, tt.data)
			assert.NoError(t, err, "rendering with empty data must not panic")
		})
	}
}
