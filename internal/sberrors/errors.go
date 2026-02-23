package sberrors

import (
	"errors"
	"fmt"
)

// ErrorCode represents a structured error category used throughout the system.
type ErrorCode string

const (
	ErrCodeFileNotFound        ErrorCode = "FILE_NOT_FOUND"
	ErrCodeMetadataNotFound    ErrorCode = "METADATA_NOT_FOUND"
	ErrCodeMetadataExists      ErrorCode = "METADATA_EXISTS"
	ErrCodePathTraversal       ErrorCode = "PATH_TRAVERSAL"
	ErrCodeNotMarkdown         ErrorCode = "NOT_MARKDOWN"
	ErrCodeAlreadyInitialized  ErrorCode = "ALREADY_INITIALIZED"
	ErrCodeNotInitialized      ErrorCode = "NOT_INITIALIZED"
	ErrCodeRebuildInProgress   ErrorCode = "REBUILD_IN_PROGRESS"
	ErrCodeNoEmbeddingProvider ErrorCode = "NO_EMBEDDING_PROVIDER"
	ErrCodeInvalidInput        ErrorCode = "INVALID_INPUT"
	ErrCodeDatabaseError       ErrorCode = "DATABASE_ERROR"
	ErrCodeLedgerError         ErrorCode = "LEDGER_ERROR"
	ErrCodeEmbeddingError      ErrorCode = "EMBEDDING_ERROR"
	ErrCodeSchemaVersion       ErrorCode = "SCHEMA_VERSION_MISMATCH"
	ErrCodeInternalError       ErrorCode = "INTERNAL_ERROR"
)

// Error is a structured error with a code, message, and optional wrapped cause.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// New creates a new Error with no wrapped cause.
func New(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Newf creates a new Error with a formatted message and no wrapped cause.
func Newf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates a new Error wrapping an existing error.
// Returns nil if err is nil.
func Wrap(err error, code ErrorCode, message string) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Wrapf creates a new Error wrapping an existing error with a formatted message.
// Returns nil if err is nil.
func Wrapf(err error, code ErrorCode, format string, args ...any) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Err:     err,
	}
}

// HasCode reports whether any error in err's chain has the given ErrorCode.
func HasCode(err error, code ErrorCode) bool {
	var e *Error
	for {
		if !errors.As(err, &e) {
			return false
		}
		if e.Code == code {
			return true
		}
		err = e.Err
	}
}
