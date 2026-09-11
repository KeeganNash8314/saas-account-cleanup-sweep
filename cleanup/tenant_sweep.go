package cleanup

import "time"

type Lifecycle string

const (
	Onboarding Lifecycle = "onboarding"
	Active     Lifecycle = "active"
	Suspended  Lifecycle = "suspended"
	Closed     Lifecycle = "closed"
)

type Tenant struct {
	ID        string    `json:"id"`
	Lifecycle Lifecycle `json:"lifecycle"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Decision struct {
	TenantID string `json:"tenant_id"`
	Delete   bool   `json:"delete"`
	Reason   string `json:"reason"`
}

// Sweep returns an auditable decision for every tenant. The caller owns deletion.
func Sweep(tenants []Tenant, now time.Time, staleAfter time.Duration) []Decision {
	cutoff := now.Add(-staleAfter)
	decisions := make([]Decision, 0, len(tenants))
	for _, tenant := range tenants {
		decision := Decision{TenantID: tenant.ID}
		switch {
		case tenant.Lifecycle == Active:
			decision.Reason = "active account"
		case tenant.UpdatedAt.After(cutoff):
			decision.Reason = "lifecycle change is inside retention window"
		case tenant.Lifecycle == Onboarding:
			decision.Delete = true
			decision.Reason = "onboarding expired"
		case tenant.Lifecycle == Suspended:
			decision.Delete = true
			decision.Reason = "suspension expired"
		case tenant.Lifecycle == Closed:
			decision.Delete = true
			decision.Reason = "closure retention expired"
		default:
			decision.Reason = "unrecognized lifecycle"
		}
		decisions = append(decisions, decision)
	}
	return decisions
}
