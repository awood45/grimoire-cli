package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetect_claudeCodePresent verifies that DetectPlatforms returns
// PlatformClaudeCode when the ~/.claude/ directory exists (FR-3.1.2).
func TestDetect_claudeCodePresent(t *testing.T) {
	homeDir := t.TempDir()
	claudeDir := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	d := NewDetector(homeDir)
	platforms := d.DetectPlatforms()

	assert.Contains(t, platforms, PlatformClaudeCode)
}

// TestDetect_claudeCodeAbsent verifies that DetectPlatforms returns an empty
// slice when the ~/.claude/ directory does not exist (FR-3.1.2).
func TestDetect_claudeCodeAbsent(t *testing.T) {
	homeDir := t.TempDir()

	d := NewDetector(homeDir)
	platforms := d.DetectPlatforms()

	assert.Empty(t, platforms)
}

// TestInstallSkills_claudeCode verifies that InstallSkills writes the skill
// file and appends the CLAUDE.md snippet for Claude Code (FR-3.1.2).
func TestInstallSkills_claudeCode(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.NoError(t, err)

	// Verify skill file was written.
	skillPath := filepath.Join(homeDir, ".claude", "commands", "write-to-grimoire.md")
	skillContent, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Contains(t, string(skillContent), brainPath)
	// Verify no template placeholders remain.
	assert.NotContains(t, string(skillContent), "{{.BasePath}}")
	assert.NotContains(t, string(skillContent), "{{if ")
	assert.NotContains(t, string(skillContent), "{{end}}")

	// Verify CLAUDE.md was updated.
	claudeMDPath := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	claudeContent, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)
	assert.Contains(t, string(claudeContent), brainPath)
	assert.Contains(t, string(claudeContent), "grimoire-cli")
}

// TestInstallSkills_unknownPlatform verifies that InstallSkills is a no-op
// for unrecognized platforms (FR-3.1.2).
func TestInstallSkills_unknownPlatform(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{Platform("unknown-platform")})
	require.NoError(t, err)

	// Verify no files were created in the home directory.
	entries, err := os.ReadDir(homeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestInstallSkills_appendsSnippetOnlyOnce verifies that calling InstallSkills
// twice does not duplicate the CLAUDE.md snippet (FR-3.1.2).
func TestInstallSkills_appendsSnippetOnlyOnce(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)

	// First installation.
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.NoError(t, err)

	claudeMDPath := filepath.Join(homeDir, ".claude", "CLAUDE.md")
	firstContent, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)

	// Second installation.
	err = d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.NoError(t, err)

	secondContent, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)

	// Content should be identical — no duplicated snippet.
	assert.Equal(t, string(firstContent), string(secondContent))
}

// TestInstallSkills_existingCLAUDEmd verifies that InstallSkills appends to
// an existing CLAUDE.md without overwriting its content (FR-3.1.2).
func TestInstallSkills_existingCLAUDEmd(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Pre-create CLAUDE.md with existing content.
	claudeDir := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	existingContent := "# Existing Project Config\n\nSome important rules here.\n"
	require.NoError(t, os.WriteFile(claudeMDPath, []byte(existingContent), 0o644))

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.NoError(t, err)

	content, err := os.ReadFile(claudeMDPath)
	require.NoError(t, err)

	// Existing content should be preserved.
	assert.True(t, strings.HasPrefix(string(content), existingContent))
	// New snippet should be appended.
	assert.Contains(t, string(content), brainPath)
}

// TestInstallSkills_emptyPlatformSlice verifies that InstallSkills with an
// empty slice is a no-op (FR-3.1.2).
func TestInstallSkills_emptyPlatformSlice(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{})
	require.NoError(t, err)

	// Verify no files were created.
	entries, err := os.ReadDir(homeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestInstallSkills_nilPlatformSlice verifies that InstallSkills with a nil
// slice is a no-op (FR-3.1.2).
func TestInstallSkills_nilPlatformSlice(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, nil)
	require.NoError(t, err)
}

// TestInstallSkills_mkdirError verifies that InstallSkills returns an error
// when the commands directory cannot be created (FR-3.1.2).
func TestInstallSkills_mkdirError(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Create a file where .claude directory would go, preventing MkdirAll.
	blockerPath := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.WriteFile(blockerPath, []byte("blocker"), 0o600))

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create commands directory")
}

// TestInstallSkills_writeSkillFileError verifies that InstallSkills returns an
// error when the skill file cannot be written (FR-3.1.2).
func TestInstallSkills_writeSkillFileError(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Create the commands directory, then place a directory where the skill
	// file should go, preventing WriteFile.
	commandsDir := filepath.Join(homeDir, ".claude", "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))
	skillBlocker := filepath.Join(commandsDir, "write-to-grimoire.md")
	require.NoError(t, os.MkdirAll(skillBlocker, 0o755))

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write skill file")
}

// TestInstallSkills_writeCLAUDEmdError verifies that InstallSkills returns an
// error when CLAUDE.md cannot be written (FR-3.1.2).
func TestInstallSkills_writeCLAUDEmdError(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Create the .claude directory, but put a directory where CLAUDE.md
	// should be, preventing WriteFile.
	claudeDir := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	claudeMDBlocker := filepath.Join(claudeDir, "CLAUDE.md")
	require.NoError(t, os.MkdirAll(claudeMDBlocker, 0o755))

	// Also create the commands dir so skill file write succeeds.
	commandsDir := filepath.Join(claudeDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.Error(t, err)
	// The error could be from ReadFile or WriteFile on CLAUDE.md.
	assert.Contains(t, err.Error(), "CLAUDE.md")
}

// TestDetect_claudeCodeFileNotDir verifies that DetectPlatforms does not
// detect Claude Code when .claude is a file rather than a directory (FR-3.1.2).
func TestDetect_claudeCodeFileNotDir(t *testing.T) {
	homeDir := t.TempDir()
	claudePath := filepath.Join(homeDir, ".claude")
	require.NoError(t, os.WriteFile(claudePath, []byte("not a dir"), 0o600))

	d := NewDetector(homeDir)
	platforms := d.DetectPlatforms()

	assert.Empty(t, platforms)
}

// TestInstallSkills_readCLAUDEmdError verifies that InstallSkills returns an
// error when CLAUDE.md exists but cannot be read (FR-3.1.2).
func TestInstallSkills_readCLAUDEmdError(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Create the .claude directory and commands dir.
	claudeDir := filepath.Join(homeDir, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Create CLAUDE.md with no read permissions.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claudeMDPath, []byte("content"), 0o000))

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CLAUDE.md")
}

// TestInstallSkills_writeClaudeMDPermissionError verifies that InstallSkills
// returns an error when the .claude directory is not writable (FR-3.1.2).
func TestInstallSkills_writeClaudeMDPermissionError(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	// Create the .claude directory and commands dir so skill file writes succeed.
	claudeDir := filepath.Join(homeDir, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Make .claude directory read-only so CLAUDE.md cannot be written.
	require.NoError(t, os.Chmod(claudeDir, 0o555))
	t.Cleanup(func() {
		// Restore permissions so t.TempDir() cleanup can delete.
		_ = os.Chmod(claudeDir, 0o755)
	})

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{PlatformClaudeCode})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.md")
}

// TestInstallSkills_multiplePlatformsIncludingClaude verifies that InstallSkills
// handles a slice with both known and unknown platforms (FR-3.1.2).
func TestInstallSkills_multiplePlatformsIncludingClaude(t *testing.T) {
	homeDir := t.TempDir()
	brainPath := filepath.Join(t.TempDir(), "grimoire-cli")
	b := brain.New(brainPath)

	d := NewDetector(homeDir)
	err := d.InstallSkills(b, []Platform{Platform("unknown"), PlatformClaudeCode})
	require.NoError(t, err)

	// Verify Claude Code skills were installed.
	skillPath := filepath.Join(homeDir, ".claude", "commands", "write-to-grimoire.md")
	_, err = os.Stat(skillPath)
	assert.NoError(t, err)
}
