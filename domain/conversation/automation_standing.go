package conversation

import "vozko/domain/actor"

type AutomationStanding struct {
	Enabled   *bool
	Delegated bool
	Owner     string
}

func (s AutomationStanding) MayAct() bool {
	if !AutomationAllowed(s.Enabled) {
		return false
	}
	return s.Delegated || !actor.IsPerson(s.Owner)
}

func (s AutomationStanding) OwnerDecides() bool {
	return AutomationAllowed(s.Enabled) && !s.Delegated
}

func (p AutomationProfile) Standing() AutomationStanding {
	return AutomationStanding{
		Enabled:   p.AutomationEnabled,
		Delegated: p.Delegate != nil && p.Delegate.Valid(),
	}
}
