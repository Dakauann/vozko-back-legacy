package conversation_usecase

import (
	"errors"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type stubRoutes struct {
	byEntry map[string]string
	err     error
}

func (s stubRoutes) RouteToDepartment(string, string, string) error { return s.err }

func (s stubRoutes) RoutedDepartment(entryID, entryType string) (string, bool, error) {
	if s.err != nil {
		return "", false, s.err
	}
	departmentID, found := s.byEntry[entryID+"|"+entryType]
	return departmentID, found, nil
}

func resolverWithNumberDepartment(routes conversation.DepartmentRoutes) conversation.CampaignWorkspaceResolver {
	resolver := NewCampaignWorkspaceResolver(nil, nil, routes)
	resolver.(ownerResolverSetter).SetEntryOwnerResolver(shared.EntryTypeUnofficialWhatsApp,
		stubOwnerResolver{workspace: "ws-1", department: "dept-number"})
	return resolver
}

func TestARoutedDepartmentWinsOverTheNumbersDepartment(t *testing.T) {
	resolver := resolverWithNumberDepartment(stubRoutes{byEntry: map[string]string{"conv-1|unofficial_whatsapp": "dept-routed"}})

	got, err := resolver.GetEntryDepartmentID("conv-1", string(shared.EntryTypeUnofficialWhatsApp))

	if err != nil || got != "dept-routed" {
		t.Fatalf("department = %q, %v, want the department it was sent to", got, err)
	}
}

func TestWithoutARouteTheNumbersDepartmentIsUsed(t *testing.T) {
	resolver := resolverWithNumberDepartment(stubRoutes{byEntry: map[string]string{"conv-1|telegram": "dept-routed"}})

	got, err := resolver.GetEntryDepartmentID("conv-1", string(shared.EntryTypeUnofficialWhatsApp))

	if err != nil || got != "dept-number" {
		t.Fatalf("department = %q, %v, want the number's department", got, err)
	}
}

func TestAnUnreadableRouteIsAnErrorNotTheDefault(t *testing.T) {
	down := errors.New("routes down")
	resolver := resolverWithNumberDepartment(stubRoutes{err: down})

	got, err := resolver.GetEntryDepartmentID("conv-1", string(shared.EntryTypeUnofficialWhatsApp))

	if !errors.Is(err, down) {
		t.Fatalf("err = %v, want the route read error", err)
	}
	if got != "" {
		t.Fatalf("department = %q, an unreadable route must not widen to the default", got)
	}
}

func TestWithoutRoutesTheDepartmentCannotBeResolved(t *testing.T) {
	resolver := resolverWithNumberDepartment(nil)

	got, err := resolver.GetEntryDepartmentID("conv-1", string(shared.EntryTypeUnofficialWhatsApp))

	if !errors.Is(err, conversation.ErrDepartmentRoutesUnavailable) {
		t.Fatalf("err = %v, want ErrDepartmentRoutesUnavailable", err)
	}
	if got != "" {
		t.Fatalf("department = %q, want none", got)
	}
}
