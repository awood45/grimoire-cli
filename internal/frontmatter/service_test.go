package frontmatter

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_noExistingFrontmatter(t *testing.T) {
	// FR-3.2.1: Injects YAML frontmatter at top of file, preserves content.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	originalContent := "# Hello World\n\nSome content here.\n"
	require.NoError(t, os.WriteFile(filePath, []byte(originalContent), 0o644))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "claude",
		Tags:        []string{"type/research", "lang/go"},
		Summary:     "A brief description",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	result := string(content)

	// Verify frontmatter block is present.
	assert.Contains(t, result, "---\n")
	assert.Contains(t, result, "source_agent: claude\n")
	assert.Contains(t, result, "- type/research\n")
	assert.Contains(t, result, "- lang/go\n")
	assert.Contains(t, result, "summary: A brief description\n")
	assert.Contains(t, result, "created_at: \"2025-01-15T10:00:00Z\"\n")
	assert.Contains(t, result, "updated_at: \"2025-01-15T10:00:00Z\"\n")

	// Verify original content is preserved after the frontmatter.
	assert.Contains(t, result, "# Hello World\n\nSome content here.\n")
}

func TestWrite_existingFrontmatter(t *testing.T) {
	// FR-3.2.2: Replaces existing frontmatter, preserves content.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	existingContent := "---\nsource_agent: old-agent\ntags:\n    - old/tag\nsummary: old summary\ncreated_at: \"2024-01-01T00:00:00Z\"\nupdated_at: \"2024-01-01T00:00:00Z\"\n---\n# Hello World\n\nSome content here.\n"
	require.NoError(t, os.WriteFile(filePath, []byte(existingContent), 0o644))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "claude",
		Tags:        []string{"type/research"},
		Summary:     "Updated summary",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	result := string(content)

	// Old frontmatter should be gone.
	assert.NotContains(t, result, "old-agent")
	assert.NotContains(t, result, "old/tag")
	assert.NotContains(t, result, "old summary")

	// New frontmatter should be present.
	assert.Contains(t, result, "source_agent: claude\n")
	assert.Contains(t, result, "- type/research\n")
	assert.Contains(t, result, "summary: Updated summary\n")

	// Original content should be preserved.
	assert.Contains(t, result, "# Hello World\n\nSome content here.\n")
}

func TestWrite_emptyFile(t *testing.T) {
	// FR-3.2.1: Writes frontmatter to empty file.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "empty.md",
		SourceAgent: "claude",
		Tags:        []string{"type/note"},
		Summary:     "Empty file",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	result := string(content)

	assert.Contains(t, result, "---\n")
	assert.Contains(t, result, "source_agent: claude\n")
}

func TestWrite_fileNotFound(t *testing.T) {
	// FR-3.2.1: Returns FILE_NOT_FOUND when file doesn't exist.
	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "nonexistent.md",
		SourceAgent: "claude",
	}

	err := svc.Write("/nonexistent/path/file.md", meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeFileNotFound))
}

func TestRead_valid(t *testing.T) {
	// FR-3.4.3: Parses all metadata fields from frontmatter.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ntags:\n    - type/research\n    - lang/go\nsummary: A brief description\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	meta, err := svc.Read(filePath)
	require.NoError(t, err)

	assert.Equal(t, "claude", meta.SourceAgent)
	assert.Equal(t, []string{"type/research", "lang/go"}, meta.Tags)
	assert.Equal(t, "A brief description", meta.Summary)
	assert.Equal(t, time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), meta.CreatedAt)
	assert.Equal(t, time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), meta.UpdatedAt)
}

func TestRead_noFrontmatter(t *testing.T) {
	// FR-3.4.3: Returns error for file without frontmatter.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Hello World\n"), 0o644))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestRead_partialFrontmatter(t *testing.T) {
	// FR-3.4.3: Handles frontmatter with optional fields missing.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	meta, err := svc.Read(filePath)
	require.NoError(t, err)

	assert.Equal(t, "claude", meta.SourceAgent)
	assert.Empty(t, meta.Tags)
	assert.Empty(t, meta.Summary)
	assert.Equal(t, time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), meta.CreatedAt)
}

func TestRead_fileNotFound(t *testing.T) {
	// FR-3.4.3: Returns FILE_NOT_FOUND error.
	svc := NewFileService()
	_, err := svc.Read("/nonexistent/path/file.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeFileNotFound))
}

func TestRemove_hasFrontmatter(t *testing.T) {
	// FR-3.2.4: Strips frontmatter, preserves content below.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ntags:\n    - type/research\nsummary: A brief description\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n# Hello World\n\nSome content here.\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	err := svc.Remove(filePath)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, "# Hello World\n\nSome content here.\n", string(result))
}

func TestRemove_noFrontmatter(t *testing.T) {
	// FR-3.2.4: No-op for file without frontmatter.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	originalContent := "# Hello World\n\nSome content here.\n"
	require.NoError(t, os.WriteFile(filePath, []byte(originalContent), 0o644))

	svc := NewFileService()
	err := svc.Remove(filePath)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, originalContent, string(result))
}

func TestWrite_specialChars(t *testing.T) {
	// NFR-7.4: YAML-special characters in tags/summary are escaped properly.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o644))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "claude: the agent",
		Tags:        []string{"tag: with colon", "tag [with] brackets", "tag {with} braces"},
		Summary:     "Summary with: colons, {braces}, [brackets], and #hashes",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	// Verify we can read back the metadata correctly (roundtrip).
	readMeta, err := svc.Read(filePath)
	require.NoError(t, err)

	assert.Equal(t, meta.SourceAgent, readMeta.SourceAgent)
	assert.Equal(t, meta.Tags, readMeta.Tags)
	assert.Equal(t, meta.Summary, readMeta.Summary)
}

func TestWrite_Read_roundtrip(t *testing.T) {
	// Verify complete write-read roundtrip preserves all fields.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o644))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "claude",
		Tags:        []string{"type/research", "lang/go"},
		Summary:     "A brief description",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	readMeta, err := svc.Read(filePath)
	require.NoError(t, err)

	assert.Equal(t, meta.SourceAgent, readMeta.SourceAgent)
	assert.Equal(t, meta.Tags, readMeta.Tags)
	assert.Equal(t, meta.Summary, readMeta.Summary)
	assert.Equal(t, meta.CreatedAt.UTC(), readMeta.CreatedAt.UTC())
	assert.Equal(t, meta.UpdatedAt.UTC(), readMeta.UpdatedAt.UTC())
}

func TestRemove_fileNotFound(t *testing.T) {
	// Remove on nonexistent file returns FILE_NOT_FOUND.
	svc := NewFileService()
	err := svc.Remove("/nonexistent/path/file.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeFileNotFound))
}

func TestRead_unreadableFile(t *testing.T) {
	// Read returns INTERNAL_ERROR for permission-denied files.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "unreadable.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o000))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestWrite_unreadableFile(t *testing.T) {
	// Write returns INTERNAL_ERROR when the file cannot be read after stat.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "unreadable.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o000))

	svc := NewFileService()
	meta := store.FileMetadata{
		SourceAgent: "claude",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}
	err := svc.Write(filePath, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRemove_unreadableFile(t *testing.T) {
	// Remove returns INTERNAL_ERROR when the file cannot be read after stat.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "unreadable.md")
	require.NoError(t, os.WriteFile(filePath, []byte("---\nfoo: bar\n---\n# Content\n"), 0o000))

	svc := NewFileService()
	err := svc.Remove(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRead_invalidYAML(t *testing.T) {
	// Read returns INTERNAL_ERROR for malformed YAML in frontmatter.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\n: invalid: yaml: [broken\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRead_invalidCreatedAtTimestamp(t *testing.T) {
	// Read returns INTERNAL_ERROR for unparseable created_at timestamp.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ncreated_at: \"not-a-timestamp\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRead_invalidUpdatedAtTimestamp(t *testing.T) {
	// Read returns INTERNAL_ERROR for unparseable updated_at timestamp.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"not-a-timestamp\"\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRead_unclosedFrontmatter(t *testing.T) {
	// Frontmatter with opening delimiter but no closing delimiter is treated as no frontmatter.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ntags:\n    - type/research\n# Content without closing delimiter\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	_, err := svc.Read(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestRemove_frontmatterOnly(t *testing.T) {
	// Remove on a file that is only frontmatter with no body content.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	svc := NewFileService()
	err := svc.Remove(filePath)
	require.NoError(t, err)

	result, err := os.ReadFile(filePath)
	require.NoError(t, err)

	// File should be empty after removing frontmatter with no body.
	assert.Equal(t, "", string(result))
}

func TestExtractFrontmatter_emptyContent(t *testing.T) {
	// Verify extractFrontmatter handles empty string input.
	yamlBlock, body, found := extractFrontmatter("")
	assert.Empty(t, yamlBlock)
	assert.Empty(t, body)
	assert.False(t, found)
}

func TestExtractFrontmatter_noOpeningDelimiter(t *testing.T) {
	// Content that does not start with --- is returned as-is.
	content := "some content\nwithout frontmatter\n"
	yamlBlock, body, found := extractFrontmatter(content)
	assert.Empty(t, yamlBlock)
	assert.Equal(t, content, body)
	assert.False(t, found)
}

func TestExtractFrontmatter_unclosedDelimiter(t *testing.T) {
	// Opening --- without closing --- means no valid frontmatter.
	content := "---\nkey: value\nno closing\n"
	yamlBlock, body, found := extractFrontmatter(content)
	assert.Empty(t, yamlBlock)
	assert.Equal(t, content, body)
	assert.False(t, found)
}

func TestWrite_preservesFilePermissions(t *testing.T) {
	// Write preserves the original file permissions.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o600))

	svc := NewFileService()
	meta := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "claude",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := svc.Write(filePath, meta)
	require.NoError(t, err)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWrite_writeFileError(t *testing.T) {
	// Write returns INTERNAL_ERROR when the file cannot be written.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(filePath, []byte("# Content\n"), 0o644))

	// Make the file read-only so ReadFile succeeds but WriteFile fails.
	require.NoError(t, os.Chmod(filePath, 0o444))
	t.Cleanup(func() {
		// Restore permissions so TempDir cleanup works.
		_ = os.Chmod(filePath, 0o644)
	})

	svc := NewFileService()
	meta := store.FileMetadata{
		SourceAgent: "claude",
		CreatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}
	err := svc.Write(filePath, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

func TestRemove_writeFileError(t *testing.T) {
	// Remove returns INTERNAL_ERROR when the file cannot be written.
	if runtime.GOOS == "windows" {
		t.Skip("file permission test not reliable on Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "---\nsource_agent: claude\ncreated_at: \"2025-01-15T10:00:00Z\"\nupdated_at: \"2025-01-15T10:00:00Z\"\n---\n# Content\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	// Make the file read-only so ReadFile succeeds but WriteFile fails.
	require.NoError(t, os.Chmod(filePath, 0o444))
	t.Cleanup(func() {
		_ = os.Chmod(filePath, 0o644)
	})

	svc := NewFileService()
	err := svc.Remove(filePath)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}
