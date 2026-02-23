package sberrors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_createsError(t *testing.T) {
	err := New(ErrCodeFileNotFound, "file not found: notes/meeting.md")

	assert.Equal(t, ErrCodeFileNotFound, err.Code)
	assert.Equal(t, "file not found: notes/meeting.md", err.Message)
	require.NoError(t, err.Err)
}

func TestWrap_wrapsUnderlying(t *testing.T) {
	underlying := fmt.Errorf("disk I/O error")
	err := Wrap(underlying, ErrCodeDatabaseError, "failed to query metadata")

	assert.Equal(t, ErrCodeDatabaseError, err.Code)
	assert.Equal(t, "failed to query metadata", err.Message)
	assert.Equal(t, underlying, err.Err)

	// errors.Unwrap should return the underlying error
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, underlying, unwrapped)
}

func TestError_stringFormat(t *testing.T) {
	err := New(ErrCodePathTraversal, "path escapes the files directory")

	expected := "PATH_TRAVERSAL: path escapes the files directory"
	assert.Equal(t, expected, err.Error())
}

func TestError_stringFormatWithWrapped(t *testing.T) {
	underlying := fmt.Errorf("connection refused")
	err := Wrap(underlying, ErrCodeEmbeddingError, "failed to generate embedding")

	expected := "EMBEDDING_ERROR: failed to generate embedding: connection refused"
	assert.Equal(t, expected, err.Error())
}

func TestError_implementsErrorInterface(t *testing.T) {
	err := New(ErrCodeInternalError, "something went wrong")

	// Should satisfy the error interface
	var e error = err
	require.Error(t, e)
	assert.Contains(t, e.Error(), "INTERNAL_ERROR")
}

func TestWrap_nilUnderlying_returnsNil(t *testing.T) {
	err := Wrap(nil, ErrCodeInvalidInput, "bad input")
	assert.Nil(t, err)
}

func TestAllErrorCodes_defined(t *testing.T) {
	// Verify all expected error codes exist and have the right string values
	codes := map[ErrorCode]string{
		ErrCodeFileNotFound:        "FILE_NOT_FOUND",
		ErrCodeMetadataNotFound:    "METADATA_NOT_FOUND",
		ErrCodeMetadataExists:      "METADATA_EXISTS",
		ErrCodePathTraversal:       "PATH_TRAVERSAL",
		ErrCodeNotMarkdown:         "NOT_MARKDOWN",
		ErrCodeAlreadyInitialized:  "ALREADY_INITIALIZED",
		ErrCodeNotInitialized:      "NOT_INITIALIZED",
		ErrCodeRebuildInProgress:   "REBUILD_IN_PROGRESS",
		ErrCodeNoEmbeddingProvider: "NO_EMBEDDING_PROVIDER",
		ErrCodeInvalidInput:        "INVALID_INPUT",
		ErrCodeDatabaseError:       "DATABASE_ERROR",
		ErrCodeLedgerError:         "LEDGER_ERROR",
		ErrCodeEmbeddingError:      "EMBEDDING_ERROR",
		ErrCodeSchemaVersion:       "SCHEMA_VERSION_MISMATCH",
		ErrCodeInternalError:       "INTERNAL_ERROR",
	}

	for code, expected := range codes {
		assert.Equal(t, expected, string(code), "ErrorCode %q should have string value %q", code, expected)
	}
}

func TestNewf_formatsMessage(t *testing.T) {
	err := Newf(ErrCodeFileNotFound, "file not found: %s", "notes/meeting.md")

	assert.Equal(t, ErrCodeFileNotFound, err.Code)
	assert.Equal(t, "file not found: notes/meeting.md", err.Message)
	require.NoError(t, err.Err)
}

func TestWrapf_formatsMessage(t *testing.T) {
	underlying := fmt.Errorf("disk I/O error")
	err := Wrapf(underlying, ErrCodeDatabaseError, "query failed for table %s", "files")

	assert.Equal(t, ErrCodeDatabaseError, err.Code)
	assert.Equal(t, "query failed for table files", err.Message)
	assert.Equal(t, underlying, err.Err)
}

func TestWrapf_nilUnderlying_returnsNil(t *testing.T) {
	err := Wrapf(nil, ErrCodeDatabaseError, "query failed for table %s", "files")
	assert.Nil(t, err)
}

func TestHasCode_directMatch(t *testing.T) {
	err := New(ErrCodeFileNotFound, "not found")
	assert.True(t, HasCode(err, ErrCodeFileNotFound))
	assert.False(t, HasCode(err, ErrCodeDatabaseError))
}

func TestHasCode_wrappedWithFmtErrorf(t *testing.T) {
	inner := New(ErrCodeFileNotFound, "not found")
	wrapped := fmt.Errorf("operation failed: %w", inner)

	assert.True(t, HasCode(wrapped, ErrCodeFileNotFound))
	assert.False(t, HasCode(wrapped, ErrCodeDatabaseError))
}

func TestHasCode_nilError(t *testing.T) {
	assert.False(t, HasCode(nil, ErrCodeFileNotFound))
}

func TestHasCode_nonAppError(t *testing.T) {
	err := fmt.Errorf("plain error")
	assert.False(t, HasCode(err, ErrCodeFileNotFound))
}

func TestError_errorsAs(t *testing.T) {
	underlying := New(ErrCodeFileNotFound, "not found")
	wrapped := fmt.Errorf("operation failed: %w", underlying)

	var appErr *Error
	require.ErrorAs(t, wrapped, &appErr)
	assert.Equal(t, ErrCodeFileNotFound, appErr.Code)
}
