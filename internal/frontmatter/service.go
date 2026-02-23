// Package frontmatter provides YAML frontmatter parsing, injection, and removal for markdown files.
package frontmatter

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	"gopkg.in/yaml.v3"
)

// Service defines operations for reading, writing, and removing YAML frontmatter from markdown files.
type Service interface {
	Read(absPath string) (store.FileMetadata, error)
	Write(absPath string, meta store.FileMetadata) error
	Remove(absPath string) error
}

// frontmatterData is the internal YAML representation of file metadata.
type frontmatterData struct {
	SourceAgent string   `yaml:"source_agent"`
	Tags        []string `yaml:"tags,omitempty"`
	Summary     string   `yaml:"summary,omitempty"`
	CreatedAt   string   `yaml:"created_at"`
	UpdatedAt   string   `yaml:"updated_at"`
}

// FileService implements the Service interface using the local filesystem.
type FileService struct{}

// NewFileService creates a new FileService.
func NewFileService() *FileService {
	return &FileService{}
}

// Read opens the file at absPath, detects YAML frontmatter, and parses it into FileMetadata.
func (s *FileService) Read(absPath string) (store.FileMetadata, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeFileNotFound, "file not found: %s", absPath)
		}
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read file")
	}

	yamlBlock, _, found := extractFrontmatter(string(data))
	if !found {
		return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeInvalidInput, "no frontmatter found in file: %s", absPath)
	}

	var fm frontmatterData
	if unmarshalErr := yaml.Unmarshal([]byte(yamlBlock), &fm); unmarshalErr != nil {
		return store.FileMetadata{}, sberrors.Wrap(unmarshalErr, sberrors.ErrCodeInternalError, "failed to parse frontmatter YAML")
	}

	meta, convertErr := fromFrontmatterData(&fm)
	if convertErr != nil {
		return store.FileMetadata{}, convertErr
	}

	return meta, nil
}

// Write reads the existing file content, prepends or replaces frontmatter, and writes it back.
func (s *FileService) Write(absPath string, meta store.FileMetadata) error { //nolint:gocritic // FileMetadata passed by value for interface consistency.
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return sberrors.Newf(sberrors.ErrCodeFileNotFound, "file not found: %s", absPath)
		}
		return sberrors.Wrap(statErr, sberrors.ErrCodeInternalError, "failed to stat file")
	}
	perm := info.Mode().Perm()

	data, err := os.ReadFile(absPath)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read file")
	}

	_, body, _ := extractFrontmatter(string(data))

	fm := toFrontmatterData(&meta)
	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal frontmatter YAML")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	buf.WriteString(body)

	if writeErr := os.WriteFile(absPath, buf.Bytes(), perm); writeErr != nil {
		return sberrors.Wrap(writeErr, sberrors.ErrCodeInternalError, "failed to write file")
	}

	return nil
}

// Remove strips YAML frontmatter from the file, preserving all content below it.
func (s *FileService) Remove(absPath string) error {
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return sberrors.Newf(sberrors.ErrCodeFileNotFound, "file not found: %s", absPath)
		}
		return sberrors.Wrap(statErr, sberrors.ErrCodeInternalError, "failed to stat file")
	}
	perm := info.Mode().Perm()

	data, err := os.ReadFile(absPath)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read file")
	}

	_, body, found := extractFrontmatter(string(data))
	if !found {
		// No frontmatter to remove — no-op.
		return nil
	}

	if writeErr := os.WriteFile(absPath, []byte(body), perm); writeErr != nil {
		return sberrors.Wrap(writeErr, sberrors.ErrCodeInternalError, "failed to write file")
	}

	return nil
}

// extractFrontmatter splits file content into the YAML block and the remaining body.
// Returns the YAML content (without delimiters), the body after the closing delimiter,
// and a boolean indicating whether frontmatter was found.
func extractFrontmatter(content string) (yamlBlock, body string, found bool) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// The first line must be "---".
	if !scanner.Scan() || strings.TrimRight(scanner.Text(), " \t") != "---" {
		return "", content, false
	}

	var yamlLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimRight(line, " \t") == "---" {
			// Found closing delimiter.
			block := strings.Join(yamlLines, "\n") + "\n"

			// Collect the remaining body.
			var bodyLines []string
			for scanner.Scan() {
				bodyLines = append(bodyLines, scanner.Text())
			}

			remaining := ""
			if len(bodyLines) > 0 {
				remaining = strings.Join(bodyLines, "\n") + "\n"
			}

			return block, remaining, true
		}
		yamlLines = append(yamlLines, line)
	}

	// No closing delimiter found — not valid frontmatter.
	return "", content, false
}

// toFrontmatterData converts FileMetadata to the internal YAML struct.
func toFrontmatterData(meta *store.FileMetadata) frontmatterData {
	return frontmatterData{
		SourceAgent: meta.SourceAgent,
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		CreatedAt:   meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   meta.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// fromFrontmatterData converts the internal YAML struct to FileMetadata.
func fromFrontmatterData(fm *frontmatterData) (store.FileMetadata, error) {
	createdAt, err := time.Parse(time.RFC3339, fm.CreatedAt)
	if err != nil {
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to parse created_at timestamp")
	}

	updatedAt, err := time.Parse(time.RFC3339, fm.UpdatedAt)
	if err != nil {
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to parse updated_at timestamp")
	}

	return store.FileMetadata{
		SourceAgent: fm.SourceAgent,
		Tags:        fm.Tags,
		Summary:     fm.Summary,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}
