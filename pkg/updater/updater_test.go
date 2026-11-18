package updater

import (
	"os"
	"testing"
)

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

func TestCleanRestartArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"shadowstar.exe", "-c", "custom.json", "-update", "--update", "-tun"}
	clean := cleanRestartArgs()

	expected := []string{"-c", "custom.json", "-tun"}
	if len(clean) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(clean), clean)
	}
	for i := range expected {
		if clean[i] != expected[i] {
			t.Errorf("at index %d: expected %q, got %q", i, expected[i], clean[i])
		}
	}
}
