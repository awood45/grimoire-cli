package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteSuccess_format verifies the JSON envelope matches {"ok": true, "data": {...}} (NFR-6.5).
func TestWriteSuccess_format(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := map[string]string{"filepath": "notes/test.md"}
	WriteSuccess(&buf, data)

	var envelope map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, true, envelope["ok"])
	assert.NotNil(t, envelope["data"])

	dataMap, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "notes/test.md", dataMap["filepath"])
}

// TestWriteSuccess_nestedData verifies complex data structures serialize properly (NFR-6.5).
func TestWriteSuccess_nestedData(t *testing.T) {
	t.Parallel()

	type nested struct {
		Name  string   `json:"name"`
		Tags  []string `json:"tags"`
		Count int      `json:"count"`
	}

	var buf bytes.Buffer
	data := nested{
		Name:  "test",
		Tags:  []string{"a", "b"},
		Count: 42,
	}
	WriteSuccess(&buf, data)

	var envelope map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, true, envelope["ok"])

	dataMap, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test", dataMap["name"])
	assert.InDelta(t, 42, dataMap["count"], 0.001)

	tags, ok := dataMap["tags"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"a", "b"}, tags)
}

// TestWriteSuccess_nilData verifies nil data produces {"ok": true, "data": null}.
func TestWriteSuccess_nilData(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	WriteSuccess(&buf, nil)

	var envelope map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, true, envelope["ok"])
	assert.Nil(t, envelope["data"])
}

// TestWriteError_format verifies the error envelope matches {"ok": false, "error": {"code": "...", "message": "..."}} (NFR-6.5).
func TestWriteError_format(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := sberrors.New(sberrors.ErrCodeFileNotFound, "File not found: notes/missing.md")
	WriteError(&buf, err)

	var envelope map[string]interface{}
	parseErr := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, parseErr)

	assert.Equal(t, false, envelope["ok"])

	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "FILE_NOT_FOUND", errObj["code"])
	assert.Equal(t, "File not found: notes/missing.md", errObj["message"])
}

// TestWriteError_wrappedError verifies that a wrapped sberrors.Error extracts code and message.
func TestWriteError_wrappedError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	inner := fmt.Errorf("disk full")
	err := sberrors.Wrap(inner, sberrors.ErrCodeDatabaseError, "failed to write")
	WriteError(&buf, err)

	var envelope map[string]interface{}
	parseErr := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, parseErr)

	assert.Equal(t, false, envelope["ok"])

	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "DATABASE_ERROR", errObj["code"])
	assert.Equal(t, "failed to write", errObj["message"])
}

// TestWriteSuccessWithWarning_format verifies the warning envelope matches {"ok": true, "data": {...}, "warning": "..."} (FR-11).
func TestWriteSuccessWithWarning_format(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := map[string]string{"filepath": "notes/test.md"}
	WriteSuccessWithWarning(&buf, data, "embedding generation failed")

	var envelope map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)

	assert.Equal(t, true, envelope["ok"])
	assert.NotNil(t, envelope["data"])
	assert.Equal(t, "embedding generation failed", envelope["warning"])

	dataMap, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "notes/test.md", dataMap["filepath"])
}

// TestWriteError_unknownError verifies non-*sberrors.Error is wrapped as INTERNAL_ERROR (NFR-6.5).
func TestWriteError_unknownError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := fmt.Errorf("something unexpected")
	WriteError(&buf, err)

	var envelope map[string]interface{}
	parseErr := json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, parseErr)

	assert.Equal(t, false, envelope["ok"])

	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
	assert.Equal(t, "something unexpected", errObj["message"])
}
