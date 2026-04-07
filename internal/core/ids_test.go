package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWorkflowID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// Valid
		{"simple", "my-workflow", false},
		{"argus-reserved", "argus-init", false},
		{"single-segment", "release", false},
		{"digit-start", "123-workflow", false},
		{"single-char", "a", false},
		// Invalid
		{"empty", "", true},
		{"uppercase", "My-Workflow", true},
		{"spaces", "my workflow", true},
		{"trailing-hyphen", "my-workflow-", true},
		{"leading-hyphen", "-my-workflow", true},
		{"double-hyphen", "a--b", true},
		{"underscore", "my_workflow", true},
		{"special-chars", "my.workflow", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflowID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidID), "expected ErrInvalidID, got: %v", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateJobID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// Valid
		{"simple", "run_tests", false},
		{"single-word", "build", false},
		{"with-numbers", "deploy2", false},
		{"multi-segment", "deploy_staging_env", false},
		// Invalid
		{"empty", "", true},
		{"digit-start", "123_bad", true},
		{"uppercase", "Run_Tests", true},
		{"hyphens", "run-tests", true},
		{"leading-underscore", "_run", true},
		{"double-underscore", "a__b", true},
		{"trailing-underscore", "run_", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJobID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidID), "expected ErrInvalidID, got: %v", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSkillName(t *testing.T) {
	tests := []struct {
		name      string
		skillName string
		wantErr   bool
	}{
		// Valid
		{"argus-skill", "argus-doctor", false},
		{"custom-skill", "my-linter", false},
		{"single-char", "a", false},
		{"max-length", strings.Repeat("a", 64), false},
		// Invalid
		{"empty", "", true},
		{"too-long", strings.Repeat("a", 65), true},
		{"with-colon", "my:skill", true},
		{"uppercase", "MySkill", true},
		{"underscore", "my_skill", true},
		{"double-hyphen", "a--b", true},
		{"trailing-hyphen", "skill-", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSkillName(tt.skillName)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidID), "expected ErrInvalidID, got: %v", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsArgusReserved(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"argus-init", "argus-init", true},
		{"argus-doctor", "argus-doctor", true},
		{"argus-prefix", "argus-", true},
		{"user-workflow", "my-workflow", false},
		{"release", "release", false},
		{"no-prefix", "workflow", false},
		{"partial-argus", "my-argus-workflow", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsArgusReserved(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}
