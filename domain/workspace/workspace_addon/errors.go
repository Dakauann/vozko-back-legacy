package workspace_addon

import "errors"

var (
	ErrAddonNotFound                 = errors.New("addon not found")
	ErrAddonSubscriptionNotFound     = errors.New("addon subscription not found")
	ErrInvalidAddonKey               = errors.New("addon key is required")
	ErrInvalidAddonName              = errors.New("addon name is required")
	ErrInvalidEntitlementKind        = errors.New("invalid entitlement kind")
	ErrInvalidUnitsPerQuantity       = errors.New("units per quantity must be positive")
	ErrInvalidAddonPrice             = errors.New("addon price and cost must be non-negative")
	ErrInvalidQuantity               = errors.New("quantity must be positive")
	ErrAddonInactive                 = errors.New("addon is not available for purchase")
	ErrAddonArchived                 = errors.New("addon is archived")
	ErrAddonKeyExists                = errors.New("addon key already exists")
	ErrInvalidAddonSubscription      = errors.New("invalid addon subscription")
	ErrWhatsAppPhoneLimitReached     = errors.New("whatsapp business phone limit reached for this workspace")
	ErrInvalidMaxUnits               = errors.New("the unit cap must be zero or at least one quantity")
	ErrMaxUnitsRequired              = errors.New("this add-on kind needs a unit cap per workspace")
	ErrAddonUnitCapReached           = errors.New("the workspace reached the most units this add-on allows")
	ErrUnitsInUse                    = errors.New("the workspace uses more units than the new quantity")
	ErrReductionUnsupported          = errors.New("this add-on kind cannot be reduced yet")
	ErrQuantityIncreaseNeedsPurchase = errors.New("more units are bought, not scheduled")
	ErrMonthlyOnly                   = errors.New("this add-on kind is sold monthly only")
	ErrMonthlyPriceRequired          = errors.New("this add-on kind needs a monthly price above zero")
	ErrPurchaseChargeInvalid         = errors.New("a priced add-on cannot be bought without a positive charge")
	ErrPurchaseAlreadyRecorded       = errors.New("this purchase reference is already in the ledger, so nothing was granted")
)
