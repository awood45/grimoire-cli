package store

import (
	"encoding/binary"
	"math"
)

// EncodeVector converts a float32 slice to a little-endian byte slice.
// Each float32 occupies 4 bytes. Returns nil for nil or empty input.
func EncodeVector(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DecodeVector converts a little-endian byte slice back to a float32 slice.
// Each 4 bytes are interpreted as one float32. Returns nil for nil or empty input.
func DecodeVector(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
