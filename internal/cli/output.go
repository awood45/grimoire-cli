package cli

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// successEnvelope is the JSON envelope for successful responses.
type successEnvelope struct {
	OK   bool        `json:"ok"`
	Data interface{} `json:"data"`
}

// errorEnvelope is the JSON envelope for error responses.
type errorEnvelope struct {
	OK    bool        `json:"ok"`
	Error errorDetail `json:"error"`
}

// errorDetail holds the structured error fields.
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteSuccess writes a JSON success envelope to w.
func WriteSuccess(w io.Writer, data interface{}) {
	writeJSON(w, successEnvelope{OK: true, Data: data})
}

// successWithWarningEnvelope is the JSON envelope for successful responses that include a warning.
type successWithWarningEnvelope struct {
	OK      bool        `json:"ok"`
	Data    interface{} `json:"data"`
	Warning string      `json:"warning"`
}

// WriteSuccessWithWarning writes a JSON success envelope with an additional warning field.
// Used when an operation succeeds but a non-fatal issue occurred (e.g., embedding generation failed).
func WriteSuccessWithWarning(w io.Writer, data interface{}, warning string) {
	writeJSON(w, successWithWarningEnvelope{OK: true, Data: data, Warning: warning})
}

// WriteError writes a JSON error envelope to w.
// If err is an *sberrors.Error, its code and message are used.
// Otherwise, the error is wrapped as INTERNAL_ERROR.
func WriteError(w io.Writer, err error) {
	code := string(sberrors.ErrCodeInternalError)
	message := err.Error()

	var sbErr *sberrors.Error
	if errors.As(err, &sbErr) {
		code = string(sbErr.Code)
		message = sbErr.Message
	}

	writeJSON(w, errorEnvelope{
		OK:    false,
		Error: errorDetail{Code: code, Message: message},
	})
}

// writeJSON encodes v as JSON to w. Encoding errors are silently
// dropped because there is no recovery path when writing CLI output fails.
func writeJSON(w io.Writer, v interface{}) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	//nolint:errcheck // No recovery path for CLI output write failures.
	enc.Encode(v)
}
