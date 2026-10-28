package siteserve_usecase

import (
	"context"
	"log"
	"net/http"

	"vozko/domain/sitepublish"
)

type pick struct {
	file    string
	variant string
	fresh   bool
}

func (s *Service) content(ctx context.Context, v visit, path string) Outcome {
	m, ok := s.manifest(ctx, v)
	if !ok {
		return Outcome{Kind: KindUnavailable}
	}
	res := m.Resolve(path)
	chosen := s.pickVariant(v, m, res)
	if chosen.file == "" {
		return Outcome{Kind: KindNotFound}
	}
	entry := m.Files[chosen.file]
	body, err := s.files.Blob(ctx, entry.SHA256)
	if err != nil {
		log.Printf("[siteserve] file %s of deployment %s: %v", chosen.file, m.DeploymentID, err)
		return Outcome{Kind: KindUnavailable}
	}
	out := Outcome{Kind: KindContent, Content: Content{
		Body: body, ContentType: entry.ContentType, Policy: m.ContentSecurityPolicy(), Missing: !res.Found, NoIndex: v.readOnly,
	}}
	if !entry.IsHTML() {
		out.Content.CacheControl = entry.CacheControl(false)
		return out
	}
	out.Content.CacheControl = entry.CacheControl(chosen.variant != "" || v.settings.PasswordHash != "")
	out.Content.ETag = entry.ETag()
	if chosen.fresh {
		out.Cookies = []Cookie{{Name: VariantCookie, Value: chosen.variant, Path: res.Route, MaxAge: VariantCookieLifetime}}
	}
	if res.Found && res.Route != "" && entry.CountView && !v.readOnly && v.Method == http.MethodGet && !sitepublish.IsBot(v.UserAgent) {
		s.countView(ctx, v, m, res.Route, chosen.variant)
	}
	if v.Method == http.MethodHead {
		return out
	}
	page, err := sitepublish.PersonalizePage(body, chosen.variant, s.queryFor(v))
	if err != nil {
		log.Printf("[siteserve] personalize %s of deployment %s: %v", chosen.file, m.DeploymentID, err)
		return Outcome{Kind: KindUnavailable}
	}
	out.Content.Body = page
	return out
}

func (s *Service) pickVariant(v visit, m sitepublish.DeploymentManifest, res sitepublish.Resolution) pick {
	if res.Route == "" {
		return pick{file: res.File}
	}
	experiment, ok := v.settings.ExperimentFor(res.Route)
	if !ok {
		return pick{file: res.File}
	}
	key, fresh := experiment.Pick(v.Cookie(VariantCookie), s.roll)
	file, key := m.VariantFile(res.Route, key)
	return pick{file: file, variant: key, fresh: fresh}
}

func (s *Service) queryFor(v visit) func(string) string {
	if sitepublish.IsBot(v.UserAgent) {
		return func(string) string { return "" }
	}
	return v.query.Get
}

func (s *Service) countView(ctx context.Context, v visit, m sitepublish.DeploymentManifest, route, variant string) {
	if variant == "" {
		variant = sitepublish.ControlVariant
	}
	visit := s.visitOf(v, m, sitepublish.VisitView, route, variant, sitepublish.ReferrerOrigin(v.Referer), sitepublish.UTMValue(v.query.Get("utm_source")))
	if err := s.record(ctx, visit); err != nil {
		log.Printf("[siteserve] view of %s not counted: %v", m.ProjectID, err)
	}
}

func (s *Service) visitOf(v visit, m sitepublish.DeploymentManifest, event sitepublish.VisitEvent, route, variant, referrer, utm string) sitepublish.Visit {
	day := sitepublish.AnalyticsDay(s.now())
	return sitepublish.Visit{
		ProjectID: m.ProjectID, Day: day, Route: route, Variant: variant, Event: event, Referrer: referrer, UTMSource: utm,
		Visitor: sitepublish.VisitorHash(s.cfg.CookieSecret, day, v.ClientIP, v.UserAgent, m.ProjectID),
	}
}

func (s *Service) record(ctx context.Context, visit sitepublish.Visit) error {
	if err := visit.Validate(); err != nil {
		return err
	}
	return s.visits.Record(ctx, visit)
}
