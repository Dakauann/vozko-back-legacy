package attendance_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"vozko/domain/attendance"
	"vozko/domain/balance"
	"vozko/domain/cache"
	"vozko/domain/shared/deps"
	"vozko/domain/workspace/workspace_pricing"
)

type ServiceMessagePrices interface {
	ServiceMessagePrice(workspaceID string) (workspace_pricing.PriceResult, error)
}

type ServiceMessagesDeps struct {
	Reader  attendance.ServiceMessageReader
	Prices  ServiceMessagePrices
	Rates   balance.ExchangeRate
	Caching SectionCaching
}

type ServiceMessageSections struct {
	reader  attendance.ServiceMessageReader
	prices  ServiceMessagePrices
	rates   balance.ExchangeRate
	caching SectionCaching
}

var _ attendance.ServiceMessageSectionsUseCase = (*ServiceMessageSections)(nil)

func NewServiceMessageSections(d ServiceMessagesDeps) (*ServiceMessageSections, error) {
	if err := deps.Require("attendance service messages", map[string]any{
		"service message reader": d.Reader,
		"service message prices": d.Prices,
		"exchange rate":          d.Rates,
		"section memo":           d.Caching.Memo,
		"analytics gate":         d.Caching.Gate,
		"section versions":       d.Caching.Versions,
	}); err != nil {
		return nil, err
	}
	if d.Caching.TTL <= 0 {
		return nil, ErrSectionTTLRequired
	}
	return &ServiceMessageSections{reader: d.Reader, prices: d.Prices, rates: d.Rates, caching: d.Caching}, nil
}

func (s *ServiceMessageSections) chargedPrice(workspaceID string) (int64, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return 0, attendance.ErrServiceMessagesNotOnPlan
	}
	price, err := s.prices.ServiceMessagePrice(workspaceID)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", attendance.ErrServiceMessagePriceUnavailable, err)
	}
	if !attendance.ChargesServiceMessages(price.PriceMicros) {
		return 0, attendance.ErrServiceMessagesNotOnPlan
	}
	return price.PriceMicros, nil
}

func (s *ServiceMessageSections) AvailableSections(_ context.Context, workspaceID string) []attendance.Section {
	_, err := s.chargedPrice(workspaceID)
	if err != nil && !errors.Is(err, attendance.ErrServiceMessagesNotOnPlan) {
		log.Printf("[attendance] service messages hidden for workspace %s: %v", workspaceID, err)
	}
	return attendance.AvailableSections(err == nil)
}

func (s *ServiceMessageSections) ServiceMessages(
	ctx context.Context,
	workspaceID string,
	q attendance.ServiceMessageQuery,
) (*attendance.ServiceMessagesSection, error) {
	price, err := s.chargedPrice(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if !q.Applies() {
		out := attendance.NotApplicableServiceMessages()
		return &out, nil
	}
	counts, err := rememberKeyed(ctx, s.caching, workspaceID, q.Fingerprint(),
		func(ctx context.Context) ([]attendance.ServiceMessageCount, error) {
			var rows []attendance.ServiceMessageCount
			err := cache.Gated(ctx, s.caching.Gate, func(ctx context.Context) error {
				var err error
				rows, err = s.reader.ReadServiceMessages(ctx, workspaceID, q)
				return err
			})
			return rows, err
		})
	if err != nil {
		return nil, err
	}
	out := attendance.BuildServiceMessages(q, counts)
	out.PriceAt(price, s.currentRate())
	return &out, nil
}

func (s *ServiceMessageSections) currentRate() float64 {
	rate, err := s.rates.CurrentRate()
	if err != nil {
		return 0
	}
	return rate
}
