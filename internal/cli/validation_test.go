package cli

import (
	"strings"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateTags_valid verifies that well-formed tags pass validation (NFR-6.5).
func TestValidateTags_valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
	}{
		{"simple lowercase", []string{"meeting-notes"}},
		{"with slash", []string{"project/backend"}},
		{"numeric", []string{"v2"}},
		{"multiple valid", []string{"meeting-notes", "project/backend", "v2"}},
		{"single char", []string{"a"}},
		{"single digit", []string{"1"}},
		{"complex path", []string{"team/frontend/react"}},
		{"hyphenated", []string{"my-long-tag-name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTags(tt.tags)
			assert.NoError(t, err)
		})
	}
}

// TestValidateTags_invalid verifies that malformed tags are rejected (NFR-6.5).
func TestValidateTags_invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
	}{
		{"uppercase", []string{"Meeting-Notes"}},
		{"empty string", []string{""}},
		{"path traversal", []string{"../bad"}},
		{"spaces", []string{"has spaces"}},
		{"starts with hyphen", []string{"-leading"}},
		{"starts with slash", []string{"/leading"}},
		{"special chars", []string{"tag@name"}},
		{"empty list", nil},
		{"empty list explicit", []string{}},
		{"mixed valid and invalid", []string{"valid", "Invalid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTags(tt.tags)
			require.Error(t, err)
			assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
		})
	}
}

// TestValidateSourceAgent_valid verifies that well-formed agent names pass (NFR-6.5).
func TestValidateSourceAgent_valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		agent string
	}{
		{"hyphenated", "claude-code"},
		{"underscore", "my_agent"},
		{"simple", "agent1"},
		{"uppercase", "MyAgent"},
		{"mixed", "Agent-v2_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSourceAgent(tt.agent)
			assert.NoError(t, err)
		})
	}
}

// TestValidateSourceAgent_invalid verifies that malformed agent names are rejected (NFR-6.5).
func TestValidateSourceAgent_invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		agent string
	}{
		{"empty", ""},
		{"has spaces", "has spaces"},
		{"special at", "a@b"},
		{"starts with hyphen", "-leading"},
		{"starts with underscore", "_leading"},
		{"special slash", "agent/name"},
		{"special dot", "agent.name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSourceAgent(tt.agent)
			require.Error(t, err)
			assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
		})
	}
}

// TestValidateSummary_valid verifies that summaries within limit pass (NFR-6.5).
func TestValidateSummary_valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		summary string
	}{
		{"empty is valid", ""},
		{"short", "A brief summary."},
		{"exactly 1024", strings.Repeat("a", 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSummary(tt.summary)
			assert.NoError(t, err)
		})
	}
}

// TestValidateSummary_tooLong verifies that summaries exceeding 1024 chars are rejected (NFR-6.5).
func TestValidateSummary_tooLong(t *testing.T) {
	t.Parallel()

	err := ValidateSummary(strings.Repeat("a", 1025))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}
