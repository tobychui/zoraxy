package utils_test

import (
	"testing"

	"imuslab.com/zoraxy/mod/utils"

	"github.com/stretchr/testify/assert"
)

func TestSizeStringToBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1024", 1024, false},
		{"1k", 1024, false},
		{"1K", 1024, false},
		{"2kb", 2 * 1024, false},
		{"1m", 1024 * 1024, false},
		{"3mb", 3 * 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},
		{"2gb", 2 * 1024 * 1024 * 1024, false},
		{"", 0, false},
		{"  5mb  ", 5 * 1024 * 1024, false},
		{"invalid", 0, true},
		{"1tb", 1099511627776, false}, // Unknown unit returns 0, nil
		{"1.5mb", int64(1.5 * 1024 * 1024), false},
	}

	for _, tt := range tests {
		got, err := utils.SizeStringToBytes(tt.input)
		if tt.hasError {
			assert.Error(t, err, "input: %s", tt.input)
		} else {
			assert.NoError(t, err, "input: %s", tt.input)
			assert.Equal(t, tt.expected, got, "input: %s", tt.input)
		}
	}
}
