package siteserve_usecase

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

type beaconPayload struct {
	Event    string `json:"event"`
	Route    string `json:"route"`
	Variant  string `json:"variant"`
	Referrer string `json:"referrer"`
	UTM      struct {
		Source string `json:"source"`
	} `json:"utm"`
}

func (s *Service) beacon(ctx context.Context, v visit) Outcome {
	if v.Method != http.MethodPost {
		return Outcome{Kind: KindMethodNotAllowed, Allow: []string{http.MethodPost}}
	}
	if v.readOnly || sitepublish.IsBot(v.UserAgent) {
		return Outcome{Kind: KindAccepted}
	}
	if !allowed(s.beacons, v.ClientIP) {
		return Outcome{Kind: KindTooManyRequests}
	}
	if len(v.Body) > MaxBeaconBytes {
		return Outcome{Kind: KindPayloadTooLarge}
	}
	var p beaconPayload
	if err := json.Unmarshal(v.Body, &p); err != nil {
		return Outcome{Kind: KindBadRequest}
	}
	event, ok := sitepublish.ParseVisitEvent(p.Event)
	if !ok || !site.ValidRoutePath(p.Route) {
		return Outcome{Kind: KindBadRequest}
	}
	m, ok := s.manifest(ctx, v)
	if !ok {
		return Outcome{Kind: KindUnavailable}
	}
	variant := sitepublish.ControlVariant
	if sitepublish.ValidVariantKey(p.Variant) {
		variant = p.Variant
	}
	visit := s.visitOf(v, m, event, p.Route, variant, sitepublish.ReferrerOrigin(p.Referrer), sitepublish.UTMValue(p.UTM.Source))
	if err := s.record(ctx, visit); err != nil {
		log.Printf("[siteserve] beacon of %s not recorded: %v", m.ProjectID, err)
		return Outcome{Kind: KindUnavailable}
	}
	return Outcome{Kind: KindAccepted}
}

func allowed(l limiter, key string) bool {
	if key == "" {
		return false
	}
	ok, _, err := l.Allow(key)
	if err != nil {
		log.Printf("[siteserve] rate limiter: %v", err)
		return false
	}
	return ok
}
