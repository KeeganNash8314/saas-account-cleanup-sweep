package cleanup

import (
	"testing"
	"time"
)

func TestSweepLifecycleDecision(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		tenant     Tenant
		wantDelete bool
		wantReason string
	}{
		{"stale onboarding", Tenant{"new-abandoned", Onboarding, now.Add(-31 * 24 * time.Hour)}, true, "onboarding expired"},
		{"active remains", Tenant{"paying", Active, now.Add(-90 * 24 * time.Hour)}, false, "active account"},
		{"recent suspension remains", Tenant{"review", Suspended, now.Add(-7 * 24 * time.Hour)}, false, "lifecycle change is inside retention window"},
		{"old closure leaves", Tenant{"departed", Closed, now.Add(-45 * 24 * time.Hour)}, true, "closure retention expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sweep([]Tenant{tt.tenant}, now, 30*24*time.Hour)[0]
			if got.Delete != tt.wantDelete || got.Reason != tt.wantReason {
				t.Fatalf("got delete=%v reason=%q", got.Delete, got.Reason)
			}
		})
	}
}
