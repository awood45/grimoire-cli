package brain

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// Brain encapsulates the grimoire directory layout and provides
// path resolution with traversal protection.
type Brain struct {
	basePath string
}

// New creates a Brain rooted at the given base path.
func New(basePath string) *Brain {
	return &Brain{basePath: basePath}
}

// FilesDir returns the absolute path to the files/ directory.
func (b *Brain) FilesDir() string {
	return filepath.Join(b.basePath, "files")
}

// ArchiveDir returns the absolute path to the archive-files/ directory.
func (b *Brain) ArchiveDir() string {
	return filepath.Join(b.basePath, "archive-files")
}

// DBPath returns the absolute path to the SQLite database.
func (b *Brain) DBPath() string {
	return filepath.Join(b.basePath, "db", "grimoire.sqlite")
}

// LedgerPath returns the absolute path to the JSONL ledger.
func (b *Brain) LedgerPath() string {
	return filepath.Join(b.basePath, "ledger.jsonl")
}

// ConfigPath returns the absolute path to config.yaml.
func (b *Brain) ConfigPath() string {
	return filepath.Join(b.basePath, "config.yaml")
}

// DocPath returns the absolute path to grimoire.md.
func (b *Brain) DocPath() string {
	return filepath.Join(b.basePath, "grimoire.md")
}

// LockPath returns the absolute path to the advisory lock file.
func (b *Brain) LockPath() string {
	return filepath.Join(b.basePath, ".lock")
}

// ResolveFilePath validates and resolves a relative path within files/.
// Returns a PATH_TRAVERSAL error if the path escapes the directory.
func (b *Brain) ResolveFilePath(relative string) (string, error) {
	return b.resolvePath(b.FilesDir(), relative)
}

// ResolveArchivePath validates and resolves a relative path within archive-files/.
// Returns a PATH_TRAVERSAL error if the path escapes the directory.
func (b *Brain) ResolveArchivePath(relative string) (string, error) {
	return b.resolvePath(b.ArchiveDir(), relative)
}

// ValidateMarkdown checks that the relative path has a .md extension.
func (b *Brain) ValidateMarkdown(relative string) error {
	if !strings.HasSuffix(relative, ".md") {
		return sberrors.Newf(sberrors.ErrCodeNotMarkdown, "file must have .md extension: %s", relative)
	}
	return nil
}

// Exists checks if the grimoire is initialized by verifying
// that the base directory and key subdirectories exist.
func (b *Brain) Exists() bool {
	if _, err := os.Stat(b.FilesDir()); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(b.basePath, "db")); err != nil {
		return false
	}
	return true
}

// resolvePath joins baseDir with relative, canonicalizes the result,
// and verifies it stays within baseDir.
func (b *Brain) resolvePath(baseDir, relative string) (string, error) {
	// Clean the relative path to collapse .. segments.
	joined := filepath.Join(baseDir, filepath.Clean(relative))

	// First check: after cleaning, does the path stay under baseDir?
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to resolve base directory")
	}

	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to resolve joined path")
	}

	if !strings.HasPrefix(absJoined, absBase+string(filepath.Separator)) && absJoined != absBase {
		return "", sberrors.Newf(sberrors.ErrCodePathTraversal, "path escapes base directory: %s", relative)
	}

	// Second check: resolve symlinks and verify again.
	if err := b.checkSymlinkTraversal(baseDir, absJoined, relative); err != nil {
		return "", err
	}

	return absJoined, nil
}

// checkSymlinkTraversal resolves symlinks and verifies the path stays within baseDir.
// Returns nil if baseDir doesn't exist (can't evaluate symlinks).
func (b *Brain) checkSymlinkTraversal(baseDir, absJoined, relative string) error {
	realBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return nil //nolint:nilerr // Intentional: base dir doesn't exist yet, symlink check not possible.
	}

	// Walk up from the target path to find the deepest existing ancestor,
	// then rebuild the path using the real (symlink-resolved) ancestor.
	evalPath, err := evalDeepest(absJoined)
	if err != nil {
		return nil //nolint:nilerr // Can't resolve any ancestor, skip check.
	}

	if !strings.HasPrefix(evalPath, realBase+string(filepath.Separator)) && evalPath != realBase {
		return sberrors.Newf(sberrors.ErrCodePathTraversal, "path escapes base directory via symlink: %s", relative)
	}

	return nil
}

// evalDeepest resolves symlinks for the deepest existing ancestor of path,
// then appends the remaining non-existent suffix.
func evalDeepest(path string) (string, error) {
	// If the full path exists, just resolve it.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}

	// Walk up until we find an ancestor that exists.
	suffix := filepath.Base(path)
	dir := filepath.Dir(path)

	for dir != path {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		path = dir
		dir = filepath.Dir(dir)
	}

	return "", os.ErrNotExist
}
