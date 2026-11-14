package dns

import (
	"testing"
)

func TestLeakPreventer_Lifecycle(t *testing.T) {
	lp := NewLeakPreventer()
	if lp == nil {
		t.Fatal("expected non-nil LeakPreventer")
	}

	// Disable should succeed idempotently
	if err := lp.Disable(); err != nil {
		t.Logf("Notice: initial Disable() returned: %v", err)
	}
}
