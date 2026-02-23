package docgen

import (
	"testing"
	"text/template"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_withStats(t *testing.T) {
	gen, err := NewTemplateGenerator()
	require.NoError(t, err)

	data := DocData{
		TotalFiles:     10,
		TrackedFiles:   8,
		OrphanedCount:  1,
		UntrackedCount: 1,
		TagInventory: []TagEntry{
			{Name: "type/research", Count: 5},
			{Name: "lang/go", Count: 3},
		},
		AgentSummary: []AgentEntry{
			{Name: "claude", FileCount: 6},
			{Name: "cursor", FileCount: 2},
		},
		LastActivity: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
	}

	output, err := gen.Generate(&data)
	require.NoError(t, err)

	assert.Contains(t, output, "| 10 |")
	assert.Contains(t, output, "| 8 |")
	assert.Contains(t, output, "| 1 |")
	assert.Contains(t, output, "`type/research`")
	assert.Contains(t, output, "`lang/go`")
	assert.Contains(t, output, "claude")
	assert.Contains(t, output, "cursor")
	assert.Contains(t, output, "2025-06-15")
}

func TestGenerate_emptyStats(t *testing.T) {
	gen, err := NewTemplateGenerator()
	require.NoError(t, err)

	data := DocData{} // zero values, LastActivity is zero time.

	output, err := gen.Generate(&data)
	require.NoError(t, err)

	assert.Contains(t, output, "No activity recorded yet")
	assert.NotContains(t, output, "Tag Inventory")
	assert.NotContains(t, output, "Agent Activity")
}

func TestGenerate_containsStaticSections(t *testing.T) {
	gen, err := NewTemplateGenerator()
	require.NoError(t, err)

	output, err := gen.Generate(&DocData{})
	require.NoError(t, err)

	assert.Contains(t, output, "# Grimoire")
	assert.Contains(t, output, "Agent Conventions")
	assert.Contains(t, output, "Tag Style Guide")
	assert.Contains(t, output, "YAML Frontmatter Format")
	assert.Contains(t, output, "CLI Command Reference")
	assert.Contains(t, output, "grimoire-cli init")
	assert.Contains(t, output, "Directory Structure")
}

func TestGenerate_withTagsOnly(t *testing.T) {
	gen, err := NewTemplateGenerator()
	require.NoError(t, err)

	data := &DocData{
		TotalFiles:   3,
		TrackedFiles: 3,
		TagInventory: []TagEntry{{Name: "test", Count: 2}},
		LastActivity: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	output, err := gen.Generate(data)
	require.NoError(t, err)
	assert.Contains(t, output, "Tag Inventory")
	assert.Contains(t, output, "`test`")
}

func TestNewTemplateGenerator_succeeds(t *testing.T) {
	gen, err := NewTemplateGenerator()
	require.NoError(t, err)
	assert.NotNil(t, gen)
}

func TestGenerate_executeError(t *testing.T) {
	// Construct a TemplateGenerator with a template that will fail during Execute.
	// Accessing a field on an int causes a template execution error.
	tmpl := template.Must(template.New("bad").Parse("{{.TotalFiles.Bad}}"))
	gen := &TemplateGenerator{tmpl: tmpl}

	_, err := gen.Generate(&DocData{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestNewGeneratorFromBytes_parseError(t *testing.T) {
	_, err := newGeneratorFromBytes([]byte("{{invalid"))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}
