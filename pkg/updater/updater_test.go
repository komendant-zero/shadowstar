package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.3.1", "v1.3.0", 1},
		{"v1.3.0", "v1.3.0", 0},
		{"v1.2.9", "v1.3.0", -1},
		{"v2.0.0", "v1.9.9", 1},
		{"1.4.0", "v1.3.9", 1},
	}

	for _, tc := range tests {
		res := compareVersions(tc.v1, tc.v2)
		if res != tc.expected {
			t.Errorf("compareVersions(%q, %q) = %d; want %d", tc.v1, tc.v2, res, tc.expected)
		}
	}
}
