package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"v1.0.0", "1.0.0", 0},
		{"v1.1.0", "v1.0.0", 1},
		{"1.0", "1.0.0", 0},
		{"1.0.0-beta", "1.0.0", 0},
	}

	for _, tt := range tests {
		got := Compare(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("1.1.0", "1.0.0") {
		t.Error("1.1.0 should be newer than 1.0.0")
	}
	if IsNewer("1.0.0", "1.0.0") {
		t.Error("1.0.0 should not be newer than 1.0.0")
	}
	if IsNewer("0.9.0", "1.0.0") {
		t.Error("0.9.0 should not be newer than 1.0.0")
	}
}
