package store

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncodeVector_basic verifies EncodeVector produces correct byte length (FR-3.2.1).
func TestEncodeVector_basic(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	b := EncodeVector(v)
	assert.Len(t, b, 12, "3 float32 values = 12 bytes")
}

// TestDecodeVector_basic verifies DecodeVector produces correct slice length (FR-3.2.1).
func TestDecodeVector_basic(t *testing.T) {
	// 12 bytes = 3 float32 values.
	b := make([]byte, 12)
	v := DecodeVector(b)
	assert.Len(t, v, 3, "12 bytes = 3 float32 values")
}

// TestEncodeDecodeVector_roundTrip verifies that encoding then decoding produces identical values (FR-3.2.1).
func TestEncodeDecodeVector_roundTrip(t *testing.T) {
	original := []float32{1.0, -2.5, 3.14, 0.0, 100.0, -0.001}
	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)
	require.Equal(t, len(original), len(decoded))
	for i := range original {
		assert.InDelta(t, original[i], decoded[i], 1e-7, "mismatch at index %d", i)
	}
}

// TestEncodeDecodeVector_zeroVector verifies encoding/decoding of a zero vector (FR-3.2.1).
func TestEncodeDecodeVector_zeroVector(t *testing.T) {
	original := []float32{0.0, 0.0, 0.0, 0.0}
	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)
	assert.Equal(t, original, decoded)
}

// TestEncodeDecodeVector_unitVectors verifies encoding/decoding of unit vectors (FR-3.2.1).
func TestEncodeDecodeVector_unitVectors(t *testing.T) {
	tests := []struct {
		name   string
		vector []float32
	}{
		{"x-axis", []float32{1.0, 0.0, 0.0}},
		{"y-axis", []float32{0.0, 1.0, 0.0}},
		{"z-axis", []float32{0.0, 0.0, 1.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeVector(tt.vector)
			decoded := DecodeVector(encoded)
			assert.Equal(t, tt.vector, decoded)
		})
	}
}

// TestEncodeDecodeVector_negativeValues verifies encoding/decoding of negative values (FR-3.2.1).
func TestEncodeDecodeVector_negativeValues(t *testing.T) {
	original := []float32{-1.0, -0.5, -100.0, -0.001}
	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)
	assert.Equal(t, original, decoded)
}

// TestEncodeDecodeVector_largeValues verifies encoding/decoding of large float32 values (FR-3.2.1).
func TestEncodeDecodeVector_largeValues(t *testing.T) {
	original := []float32{math.MaxFloat32, -math.MaxFloat32, math.SmallestNonzeroFloat32}
	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)
	assert.Equal(t, original, decoded)
}

// TestEncodeVector_empty verifies encoding of an empty vector (FR-3.2.1).
func TestEncodeVector_empty(t *testing.T) {
	b := EncodeVector([]float32{})
	assert.Empty(t, b)
}

// TestDecodeVector_empty verifies decoding of empty bytes (FR-3.2.1).
func TestDecodeVector_empty(t *testing.T) {
	v := DecodeVector([]byte{})
	assert.Empty(t, v)
}

// TestEncodeVector_nil verifies encoding of a nil vector (FR-3.2.1).
func TestEncodeVector_nil(t *testing.T) {
	b := EncodeVector(nil)
	assert.Empty(t, b)
}

// TestDecodeVector_nil verifies decoding of nil bytes (FR-3.2.1).
func TestDecodeVector_nil(t *testing.T) {
	v := DecodeVector(nil)
	assert.Empty(t, v)
}

// TestEncodeVector_singleValue verifies encoding/decoding of a single-element vector (FR-3.2.1).
func TestEncodeVector_singleValue(t *testing.T) {
	original := []float32{42.5}
	encoded := EncodeVector(original)
	assert.Len(t, encoded, 4)
	decoded := DecodeVector(encoded)
	assert.Equal(t, original, decoded)
}

// TestEncodeVector_byteOrder verifies little-endian byte order (FR-3.2.1).
func TestEncodeVector_byteOrder(t *testing.T) {
	// 1.0 as float32 = 0x3F800000.
	// In little-endian: [0x00, 0x00, 0x80, 0x3F].
	v := []float32{1.0}
	b := EncodeVector(v)
	require.Len(t, b, 4)
	assert.Equal(t, byte(0x00), b[0])
	assert.Equal(t, byte(0x00), b[1])
	assert.Equal(t, byte(0x80), b[2])
	assert.Equal(t, byte(0x3F), b[3])
}
