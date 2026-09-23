package st3

import "testing"

func TestClamp16(t *testing.T) {
	tests := []struct {
		in   int32
		want int32
	}{
		{in: -100000, want: -0x8000},
		{in: -0x8001, want: -0x8000},
		{in: -0x8000, want: -0x8000},
		{in: 0, want: 0},
		{in: 0x7FFF, want: 0x7FFF},
		{in: 0x8000, want: 0x7FFF},
		{in: 100000, want: 0x7FFF},
	}

	for _, tt := range tests {
		if got := clamp16(tt.in); got != tt.want {
			t.Errorf("clamp16(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
