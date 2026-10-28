package workspace_addon

type EntitlementKind string

const (
	EntitlementCallChannels                EntitlementKind = "call_channels"
	EntitlementWhatsAppBusinessPhones      EntitlementKind = "whatsapp_business_phones"
	EntitlementBranches                    EntitlementKind = "branches"
	EntitlementUnofficialWhatsAppInstances EntitlementKind = "unofficial_whatsapp_instances"
	EntitlementSiteDomains                 EntitlementKind = "site_domains"
)

func (k EntitlementKind) IsValid() bool {
	switch k {
	case EntitlementCallChannels, EntitlementWhatsAppBusinessPhones, EntitlementBranches,
		EntitlementUnofficialWhatsAppInstances, EntitlementSiteDomains:
		return true
	default:
		return false
	}
}

func (k EntitlementKind) RequiresUnitCap() bool {
	return k == EntitlementSiteDomains
}

func (k EntitlementKind) MonthlyOnly() bool {
	return k.RequiresUnitCap()
}

func AllEntitlementKinds() []EntitlementKind {
	return []EntitlementKind{
		EntitlementCallChannels,
		EntitlementWhatsAppBusinessPhones,
		EntitlementBranches,
		EntitlementUnofficialWhatsAppInstances,
		EntitlementSiteDomains,
	}
}

const BoundResourceWhatsAppBusinessPhone = "whatsapp_business_phone"

type EntitlementResolver interface {
	Resolve(workspaceID string, kind EntitlementKind) (int, error)
}

type BatchEntitlementResolver interface {
	ResolveMany(workspaceIDs []string, kind EntitlementKind) (map[string]int, error)
}

type EntitlementChangeHandler interface {
	OnEntitlementReduced(workspaceID string, kind EntitlementKind) error
	OnEntitlementIncreased(workspaceID string, kind EntitlementKind) error
}
