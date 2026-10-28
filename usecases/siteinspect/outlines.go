package siteinspect

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
)

const (
	CodeNotInspected        = "not_inspected"
	CodeInspectionThrottled = "inspection_throttled"
	ReasonNotInspected      = "esta versão da página ainda não foi lida; ela é lida quando alguém que pode editar abre a página no editor"
	ReasonThrottled         = "a página acabou de ser inspecionada; tente de novo em alguns segundos"

	InspectSpacing    = 5 * time.Second
	OutlineKeep       = 25 * time.Minute
	outlineKeyPrefix  = "site:outline:"
	outlineLockPrefix = "site:outline:lock:"
)

type OutlineState interface {
	SetNX(key string, value string, ttl time.Duration) (bool, error)
	GetString(key string) (string, error)
	SetString(key string, value string, ttl time.Duration) error
}

type Inspector interface {
	Inspect(ctx context.Context, view *site_usecase.View, documents Documents) Report
}

type Outlines struct {
	Inspector Inspector
	State     OutlineState
}

type storedOutline struct {
	Stamp  string `json:"stamp"`
	Report Report `json:"report"`
}

func baseReport(view *site_usecase.View) Report {
	forms := sitepublish.FormIDs(view.Source())
	if forms == nil {
		forms = []string{}
	}
	return Report{Forms: forms, Gaps: []site.Issue{}, Routes: []site.RouteOutline{}}
}

func stampOf(view *site_usecase.View) string {
	version := int64(0)
	if view.Settings != nil {
		version = view.Settings.Version
	}
	return view.Revision.ID + ":" + strconv.FormatInt(version, 10)
}

func lasting(r Report) bool {
	return r.Known || r.ReasonCode == CodeBuildFailed || r.ReasonCode == CodeRulesBroken || r.ReasonCode == site.CodeManifestInvalid
}

func (o Outlines) cached(view *site_usecase.View) (Report, bool) {
	raw, err := o.State.GetString(outlineKeyPrefix + view.Project.ID)
	if err != nil || raw == "" {
		return Report{}, false
	}
	var stored storedOutline
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.Stamp != stampOf(view) {
		return Report{}, false
	}
	return stored.Report, true
}

func pending(view *site_usecase.View, code, reason string) Report {
	report := baseReport(view)
	report.ReasonCode, report.Reason = code, reason
	return report
}

func (o Outlines) Outline(_ context.Context, view *site_usecase.View) Report {
	if cached, hit := o.cached(view); hit {
		return cached
	}
	return pending(view, CodeNotInspected, ReasonNotInspected)
}

func (o Outlines) Submit(ctx context.Context, view *site_usecase.View, snapshot Snapshot) Report {
	acquired, err := o.State.SetNX(outlineLockPrefix+view.Project.ID, "1", InspectSpacing)
	if err != nil || !acquired {
		if cached, hit := o.cached(view); hit {
			return cached
		}
		return pending(view, CodeInspectionThrottled, ReasonThrottled)
	}
	report := o.Inspector.Inspect(ctx, view, snapshot)
	if lasting(report) {
		o.remember(view, report)
	}
	return report
}

func (o Outlines) remember(view *site_usecase.View, report Report) {
	raw, err := json.Marshal(storedOutline{Stamp: stampOf(view), Report: report})
	if err == nil {
		err = o.State.SetString(outlineKeyPrefix+view.Project.ID, string(raw), OutlineKeep)
	}
	if err != nil {
		log.Printf("[sites] the outline of %s could not be remembered: %v", view.Project.ID, err)
	}
}
