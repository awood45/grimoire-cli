package platform

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/templates"
)

// grimoireSnippetMarker is used to detect if the CLAUDE.md snippet has
// already been appended, preventing duplicates.
const grimoireSnippetMarker = "<!-- grimoire-integration -->"

// Detector defines the interface for platform detection and skill installation.
type Detector interface {
	DetectPlatforms() []Platform
	InstallSkills(b *brain.Brain, platforms []Platform) error
}

// DefaultDetector implements Detector by checking the filesystem for
// known agent platform directories.
type DefaultDetector struct {
	homeDir string
}

// NewDetector creates a DefaultDetector that looks for platforms relative
// to the given home directory.
func NewDetector(homeDir string) *DefaultDetector {
	return &DefaultDetector{homeDir: homeDir}
}

// DetectPlatforms checks for recognized agent platforms and returns those
// that are present.
func (d *DefaultDetector) DetectPlatforms() []Platform {
	var platforms []Platform

	claudeDir := filepath.Join(d.homeDir, ".claude")
	if info, err := os.Stat(claudeDir); err == nil && info.IsDir() {
		platforms = append(platforms, PlatformClaudeCode)
	}

	return platforms
}

// InstallSkills installs platform-specific skills for each detected platform.
func (d *DefaultDetector) InstallSkills(b *brain.Brain, platforms []Platform) error {
	for _, p := range platforms {
		if p == PlatformClaudeCode {
			if err := d.installClaudeCodeSkills(b); err != nil {
				return err
			}
		}
		// Unknown platforms are silently skipped.
	}
	return nil
}

// installClaudeCodeSkills writes the write-to-grimoire skill and updates CLAUDE.md.
func (d *DefaultDetector) installClaudeCodeSkills(b *brain.Brain) error {
	if err := d.writeSkillFile(b); err != nil {
		return err
	}
	return d.updateClaudeMD(b)
}

// writeSkillFile reads the embedded template, replaces the placeholder, and
// writes the skill file to ~/.claude/commands/.
func (d *DefaultDetector) writeSkillFile(b *brain.Brain) error {
	tmplContent, err := templates.FS.ReadFile("write-to-grimoire.md")
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read skill template")
	}

	// Replace the template placeholder with the actual brain base path.
	basePath := filepath.Dir(b.FilesDir())
	rendered := strings.ReplaceAll(string(tmplContent), "{{.BasePath}}", basePath)

	// Ensure the commands directory exists.
	commandsDir := filepath.Join(d.homeDir, ".claude", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to create commands directory")
	}

	skillPath := filepath.Join(commandsDir, "write-to-grimoire.md")
	if err := os.WriteFile(skillPath, []byte(rendered), 0o600); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to write skill file")
	}

	return nil
}

// updateClaudeMD appends a grimoire integration snippet to CLAUDE.md if
// it is not already present.
func (d *DefaultDetector) updateClaudeMD(b *brain.Brain) error {
	claudeDir := filepath.Join(d.homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to create .claude directory")
	}

	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	basePath := filepath.Dir(b.FilesDir())

	// Read existing content if the file exists.
	existing, err := os.ReadFile(claudeMDPath)
	if err != nil && !os.IsNotExist(err) {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read CLAUDE.md")
	}

	// If the snippet is already present, do nothing.
	if strings.Contains(string(existing), grimoireSnippetMarker) {
		return nil
	}

	snippet := buildCLAUDEMDSnippet(basePath)

	// Append to existing content (or create new file).
	content := string(existing) + snippet
	if err := os.WriteFile(claudeMDPath, []byte(content), 0o600); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to write CLAUDE.md")
	}

	return nil
}

// buildCLAUDEMDSnippet returns the markdown snippet to append to CLAUDE.md.
func buildCLAUDEMDSnippet(basePath string) string {
	return "\n" + grimoireSnippetMarker + "\n" +
		"## Grimoire\n\n" +
		"A shared knowledge base is available at `" + basePath + "`.\n\n" +
		"**Available skills:**\n" +
		"- `/write-to-grimoire` — Write a markdown file and create/update its metadata in one step.\n\n" +
		"Use `grimoire-cli` CLI commands for direct operations. " +
		"See `" + basePath + "/grimoire.md` for conventions and documentation.\n"
}
