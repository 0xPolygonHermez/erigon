package hermez_db

import (
	"bytes"
	"fmt"
	"testing"
)

func TestUintBytes(t *testing.T) {
	scenarios := []struct {
		input  uint64
		output []byte
	}{
		{0, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{1, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{256, []byte{0, 1, 0, 0, 0, 0, 0, 0}},
	}

	for _, tt := range scenarios {
		t.Run(fmt.Sprintf("Input: %v", tt.input), func(t *testing.T) {
			out := UintBytes(tt.input)
			if !bytes.Equal(out, tt.output) {
				t.Errorf("got %v, want %v", out, tt.output)
			}
		})
	}
}

func TestBytesUint(t *testing.T) {
	scenarios := []struct {
		input  []byte
		output uint64
	}{
		{[]byte{0, 0, 0, 0, 0, 0, 0, 0}, 0},
		{[]byte{1, 0, 0, 0, 0, 0, 0, 0}, 1},
		{[]byte{0, 1, 0, 0, 0, 0, 0, 0}, 256},
	}

	for _, tt := range scenarios {
		t.Run(fmt.Sprintf("Input: %v", tt.input), func(t *testing.T) {
			out := BytesUint(tt.input)
			if out != tt.output {
				t.Errorf("got %v, want %v", out, tt.output)
			}
		})
	}
}

func TestSplitKey(t *testing.T) {
	scenarios := []struct {
		input       []byte
		outputL1    uint64
		outputBatch uint64
		hasError    bool
	}{
		{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0, 0, false},
		{[]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 1, 0, false},
		{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0}, 0, 1, false},
		{[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0, 0, true},
	}

	for _, tt := range scenarios {
		t.Run(fmt.Sprintf("Input: %v", tt.input), func(t *testing.T) {
			l1, batch, err := SplitKey(tt.input)
			if tt.hasError && err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !tt.hasError && err != nil {
				t.Errorf("expected no error, got %v", err)
				return
			}
			if l1 != tt.outputL1 || batch != tt.outputBatch {
				t.Errorf("got (%v, %v), want (%v, %v)", l1, batch, tt.outputL1, tt.outputBatch)
			}
		})
	}
}
