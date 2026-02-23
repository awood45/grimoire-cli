package cli

import (
	"regexp"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// tagPattern matches valid tags: lowercase alphanumeric, hyphens, forward slashes.
// Must start with a letter or digit.
var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-/]*$`)

// sourceAgentPattern matches valid source agent names: alphanumeric, hyphens, underscores.
// Must start with a letter or digit.
var sourceAgentPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_]*$`)

// maxSummaryLength is the maximum allowed length for a summary field.
const maxSummaryLength = 1024

// ValidateTags checks that all tags are non-empty and match the required pattern.
func ValidateTags(tags []string) error {
	if len(tags) == 0 {
		return sberrors.New(sberrors.ErrCodeInvalidInput, "at least one tag is required")
	}
	for _, tag := range tags {
		if tag == "" {
			return sberrors.New(sberrors.ErrCodeInvalidInput, "tag must not be empty")
		}
		if !tagPattern.MatchString(tag) {
			return sberrors.Newf(sberrors.ErrCodeInvalidInput,
				"invalid tag %q: must match %s", tag, tagPattern.String())
		}
	}
	return nil
}

// ValidateSourceAgent checks that the source agent name is non-empty and matches the required pattern.
func ValidateSourceAgent(agent string) error {
	if agent == "" {
		return sberrors.New(sberrors.ErrCodeInvalidInput, "source agent must not be empty")
	}
	if !sourceAgentPattern.MatchString(agent) {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput,
			"invalid source agent %q: must match %s", agent, sourceAgentPattern.String())
	}
	return nil
}

// ValidateSummary checks that the summary does not exceed the maximum length.
func ValidateSummary(summary string) error {
	if len(summary) > maxSummaryLength {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput,
			"summary exceeds maximum length of %d characters (got %d)", maxSummaryLength, len(summary))
	}
	return nil
}
