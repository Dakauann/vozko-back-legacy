package workspace_addon

import (
	"math"
	"strings"
	"time"

	billing "vozko/domain/billing"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type AddonDefinition struct {
	ID                 string          `json:"id"`
	Key                string          `json:"key"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	EntitlementKind    EntitlementKind `json:"entitlementKind"`
	UnitsPerQuantity   int             `json:"unitsPerQuantity"`
	MonthlyPriceMicros int64           `json:"monthlyPriceMicros"`
	AnnualPriceMicros  int64           `json:"annualPriceMicros"`
	MonthlyCostMicros  int64           `json:"monthlyCostMicros"`
	AnnualCostMicros   int64           `json:"annualCostMicros"`
	MaxUnits           int             `json:"maxUnits"`
	IsActive           bool            `json:"isActive"`
	IsGloballyVisible  bool            `json:"isGloballyVisible"`
	ArchivedAt         *time.Time      `json:"archivedAt,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

func (a *AddonDefinition) Validate() error {
	if a == nil {
		return ErrAddonNotFound
	}
	if strings.TrimSpace(a.Key) == "" {
		return ErrInvalidAddonKey
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrInvalidAddonName
	}
	if !a.EntitlementKind.IsValid() {
		return ErrInvalidEntitlementKind
	}
	if a.UnitsPerQuantity <= 0 {
		return ErrInvalidUnitsPerQuantity
	}
	if a.MonthlyPriceMicros < 0 || a.AnnualPriceMicros < 0 || a.MonthlyCostMicros < 0 || a.AnnualCostMicros < 0 {
		return ErrInvalidAddonPrice
	}
	if a.MaxUnits < 0 || (a.MaxUnits > 0 && a.MaxUnits < a.UnitsPerQuantity) {
		return ErrInvalidMaxUnits
	}
	if a.MaxUnits == 0 && a.EntitlementKind.RequiresUnitCap() {
		return ErrMaxUnitsRequired
	}
	if a.MonthlyPriceMicros <= 0 && a.EntitlementKind.MonthlyOnly() {
		return ErrMonthlyPriceRequired
	}
	return nil
}

func (a *AddonDefinition) AllowsUnits(total int) error {
	if a.MaxUnits <= 0 && a.EntitlementKind.RequiresUnitCap() {
		return ErrMaxUnitsRequired
	}
	if a.MaxUnits > 0 && total > a.MaxUnits {
		return ErrAddonUnitCapReached
	}
	return nil
}

func (a *AddonDefinition) UnitsFor(quantity int) (int, error) {
	if quantity <= 0 || a.UnitsPerQuantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	if a.MaxUnits > 0 && quantity > a.MaxUnits {
		return 0, ErrAddonUnitCapReached
	}
	if quantity > math.MaxInt/a.UnitsPerQuantity {
		return 0, ErrInvalidQuantity
	}
	return quantity * a.UnitsPerQuantity, nil
}

func (a *AddonDefinition) PurchaseCycle(requested workspace_plan.BillingCycle) (workspace_plan.BillingCycle, error) {
	if a.EntitlementKind.MonthlyOnly() {
		if requested == "" || requested == workspace_plan.BillingCycleMonthly {
			return workspace_plan.BillingCycleMonthly, nil
		}
		return "", ErrMonthlyOnly
	}
	if !requested.IsValid() {
		return workspace_plan.BillingCycleMonthly, nil
	}
	return requested, nil
}

func (a *AddonDefinition) RequiresPayment(cycle workspace_plan.BillingCycle) bool {
	return a.EntitlementKind.MonthlyOnly() || a.PriceMicros(cycle) > 0
}

func (a *AddonDefinition) IsArchived() bool {
	return a != nil && a.ArchivedAt != nil
}

func (a *AddonDefinition) PriceMicros(cycle workspace_plan.BillingCycle) int64 {
	if cycle == workspace_plan.BillingCycleAnnual {
		return a.AnnualPriceMicros
	}
	return a.MonthlyPriceMicros
}

func (a *AddonDefinition) CostMicros(cycle workspace_plan.BillingCycle) int64 {
	if cycle == workspace_plan.BillingCycleAnnual {
		return a.AnnualCostMicros
	}
	return a.MonthlyCostMicros
}

func TotalUnits(adding int, owned []*AddonSubscription) (int, error) {
	total := adding
	for _, s := range owned {
		granted := s.GrantedUnits()
		if total > math.MaxInt-granted {
			return 0, ErrInvalidQuantity
		}
		total += granted
	}
	return total, nil
}

type AddonSubscription struct {
	ID                 string                            `json:"id"`
	WorkspaceID        string                            `json:"workspaceId"`
	AddonDefinitionID  string                            `json:"addonDefinitionId"`
	AddonKey           string                            `json:"addonKey"`
	EntitlementKind    EntitlementKind                   `json:"entitlementKind"`
	Quantity           int                               `json:"quantity"`
	UnitsPerQuantity   int                               `json:"unitsPerQuantity"`
	BillingCycle       workspace_plan.BillingCycle       `json:"billingCycle"`
	Status             workspace_plan.SubscriptionStatus `json:"status"`
	UnitPriceMicros    int64                             `json:"unitPriceMicros"`
	BoundResourceType  *string                           `json:"boundResourceType,omitempty"`
	BoundResourceID    *string                           `json:"boundResourceId,omitempty"`
	CurrentPeriodStart time.Time                         `json:"currentPeriodStart"`
	CurrentPeriodEnd   time.Time                         `json:"currentPeriodEnd"`
	ScheduledQuantity  *int                              `json:"scheduledQuantity,omitempty"`
	ScheduledFrom      *time.Time                        `json:"scheduledFrom,omitempty"`
	CancelledAt        *time.Time                        `json:"cancelledAt,omitempty"`
	CreatedAt          time.Time                         `json:"createdAt"`
	UpdatedAt          time.Time                         `json:"updatedAt"`
}

func NewAddonSubscription(id, workspaceID string, def *AddonDefinition, quantity int, cycle workspace_plan.BillingCycle, quote PurchaseQuote, now time.Time) *AddonSubscription {
	return &AddonSubscription{
		ID:                 id,
		WorkspaceID:        workspaceID,
		AddonDefinitionID:  def.ID,
		AddonKey:           def.Key,
		EntitlementKind:    def.EntitlementKind,
		Quantity:           quantity,
		UnitsPerQuantity:   def.UnitsPerQuantity,
		BillingCycle:       cycle,
		Status:             workspace_plan.SubscriptionStatusActive,
		UnitPriceMicros:    def.PriceMicros(cycle),
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   quote.PeriodEnd,
	}
}

func (s *AddonSubscription) TopUp(def *AddonDefinition, quantity int, cycle workspace_plan.BillingCycle, quote PurchaseQuote, now time.Time) {
	if s == nil {
		return
	}
	s.AddQuantity(quantity)
	s.UnitsPerQuantity = def.UnitsPerQuantity
	s.UnitPriceMicros = def.PriceMicros(cycle)
	s.BillingCycle = cycle
	if quote.PeriodEnd.After(s.CurrentPeriodEnd) {
		s.CurrentPeriodEnd = quote.PeriodEnd
	}
	s.Status = workspace_plan.SubscriptionStatusActive
	s.UpdatedAt = now
}

func (s *AddonSubscription) GrantedUnits() int {
	if s == nil {
		return 0
	}
	if s.Quantity <= 0 || s.UnitsPerQuantity <= 0 {
		return 0
	}
	return s.Quantity * s.UnitsPerQuantity
}

func (s *AddonSubscription) IsActive() bool {
	return s != nil && s.Status == workspace_plan.SubscriptionStatusActive
}

func (s *AddonSubscription) Validate() error {
	if s == nil {
		return ErrInvalidAddonSubscription
	}
	if strings.TrimSpace(s.WorkspaceID) == "" || strings.TrimSpace(s.AddonDefinitionID) == "" {
		return ErrInvalidAddonSubscription
	}
	if !s.EntitlementKind.IsValid() {
		return ErrInvalidAddonSubscription
	}
	if s.Quantity <= 0 || s.UnitsPerQuantity <= 0 {
		return ErrInvalidQuantity
	}
	if s.CurrentPeriodStart.IsZero() || s.CurrentPeriodEnd.IsZero() || !s.CurrentPeriodEnd.After(s.CurrentPeriodStart) {
		return ErrInvalidAddonSubscription
	}
	return nil
}

func (s *AddonSubscription) ExpireIfNeeded(at time.Time) bool {
	if s == nil {
		return false
	}
	if s.Status == workspace_plan.SubscriptionStatusExpired {
		return false
	}
	if at.After(s.CurrentPeriodEnd) {
		s.Status = workspace_plan.SubscriptionStatusExpired
		s.UpdatedAt = at
		return true
	}
	return false
}

func (s *AddonSubscription) DueBy(anchor time.Time) bool {
	return s != nil && billing.DueOn(s.CurrentPeriodEnd, anchor)
}

func (s *AddonSubscription) RenewForInvoice(due, paidAt time.Time, dueDay int) bool {
	if s == nil {
		return false
	}
	if s.DueBy(due) {
		start := due
		if paidAt.After(start) {
			start = paidAt
		}
		s.renewFrom(start, paidAt, dueDay)
		return true
	}
	if s.Status == workspace_plan.SubscriptionStatusExpired && s.CurrentPeriodEnd.After(paidAt) {
		s.Status = workspace_plan.SubscriptionStatusActive
		s.UpdatedAt = paidAt
		return true
	}
	return false
}

func (s *AddonSubscription) renewFrom(start, at time.Time, dueDay int) {
	quantity := s.QuantityAt(start)
	if s.ScheduledFrom != nil && !start.Before(*s.ScheduledFrom) {
		s.ScheduledQuantity, s.ScheduledFrom = nil, nil
	}
	if quantity <= 0 {
		s.Status = workspace_plan.SubscriptionStatusExpired
		s.UpdatedAt = at
		return
	}
	s.Quantity = quantity
	s.CurrentPeriodStart = start
	if s.BillingCycle == workspace_plan.BillingCycleAnnual {
		s.CurrentPeriodEnd = start.AddDate(0, 12, 0)
	} else {
		s.CurrentPeriodEnd = billing.NextAnchor(start.In(billing.LocationBRT()).AddDate(0, 0, 1), dueDay)
	}
	s.Status = workspace_plan.SubscriptionStatusActive
	s.UpdatedAt = at
}

func (s *AddonSubscription) QuantityAt(at time.Time) int {
	if s == nil {
		return 0
	}
	if s.ScheduledQuantity != nil && s.ScheduledFrom != nil && !at.Before(*s.ScheduledFrom) {
		return *s.ScheduledQuantity
	}
	return s.Quantity
}

func (s *AddonSubscription) GrantedUnitsAt(at time.Time) int {
	if s == nil || s.UnitsPerQuantity <= 0 || !at.Before(billing.AccessEnd(s.CurrentPeriodEnd)) {
		return 0
	}
	quantity := s.QuantityAt(at)
	if s.EntitlementKind.RequiresUnitCap() && s.ScheduledQuantity != nil && *s.ScheduledQuantity < quantity {
		quantity = *s.ScheduledQuantity
	}
	if quantity <= 0 {
		return 0
	}
	return quantity * s.UnitsPerQuantity
}

func (s *AddonSubscription) ChangeDate(at time.Time) time.Time {
	from := billing.FirstBillingAnchor(at, billing.DefaultEmitDay, billing.DefaultDueDay)
	if s != nil && s.CurrentPeriodEnd.After(from) {
		return s.CurrentPeriodEnd
	}
	return from
}

func (s *AddonSubscription) ScheduleQuantity(quantity int, from time.Time, unitsInUse int) error {
	if s == nil || quantity <= 0 {
		return ErrInvalidQuantity
	}
	if quantity > s.Quantity {
		return ErrQuantityIncreaseNeedsPurchase
	}
	if quantity*s.UnitsPerQuantity < unitsInUse {
		return ErrUnitsInUse
	}
	s.CancelledAt = nil
	if quantity == s.Quantity {
		s.ScheduledQuantity, s.ScheduledFrom = nil, nil
		return nil
	}
	s.ScheduledQuantity, s.ScheduledFrom = &quantity, &from
	return nil
}

func (s *AddonSubscription) ScheduleCancellation(from, now time.Time, unitsInUse int) error {
	if s == nil {
		return ErrInvalidAddonSubscription
	}
	if unitsInUse > 0 {
		return ErrUnitsInUse
	}
	none := 0
	s.ScheduledQuantity, s.ScheduledFrom = &none, &from
	s.CancelledAt = &now
	s.UpdatedAt = now
	return nil
}

func (s *AddonSubscription) AddQuantity(quantity int) {
	if s == nil || quantity <= 0 {
		return
	}
	s.Quantity += quantity
	if s.ScheduledQuantity == nil {
		return
	}
	if *s.ScheduledQuantity == 0 {
		s.ScheduledQuantity, s.ScheduledFrom, s.CancelledAt = nil, nil, nil
		return
	}
	next := *s.ScheduledQuantity + quantity
	s.ScheduledQuantity = &next
}

type PurchaseQuote struct {
	ChargeMicros    int64
	CostMicros      int64
	RecurringMicros int64
	PeriodEnd       time.Time
	Prorated        bool
	ProratedDays    int
}

func QuotePurchase(def *AddonDefinition, cycle workspace_plan.BillingCycle, quantity int, at time.Time) (PurchaseQuote, error) {
	recurring, err := timesQuantity(def.PriceMicros(cycle), quantity)
	if err != nil {
		return PurchaseQuote{}, err
	}
	fullCost, err := timesQuantity(def.CostMicros(cycle), quantity)
	if err != nil {
		return PurchaseQuote{}, err
	}
	months := cycle.PeriodMonths()
	charge, periodEnd := billing.ActivationPeriod(at, billing.DefaultEmitDay, billing.DefaultDueDay, months, recurring)
	cost, _ := billing.ActivationPeriod(at, billing.DefaultEmitDay, billing.DefaultDueDay, months, fullCost)
	if def.RequiresPayment(cycle) && charge <= 0 {
		return PurchaseQuote{}, ErrPurchaseChargeInvalid
	}
	quote := PurchaseQuote{ChargeMicros: charge, CostMicros: cost, RecurringMicros: recurring, PeriodEnd: periodEnd}
	if months == 1 {
		quote.Prorated = true
		quote.ProratedDays = billing.ActivationProRataDays(at, billing.DefaultEmitDay, billing.DefaultDueDay)
	}
	return quote, nil
}

func timesQuantity(micros int64, quantity int) (int64, error) {
	if quantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	if micros > math.MaxInt64/int64(quantity) {
		return 0, ErrInvalidQuantity
	}
	return micros * int64(quantity), nil
}
