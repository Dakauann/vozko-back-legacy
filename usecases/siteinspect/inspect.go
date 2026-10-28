package siteinspect

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siterender"
)

const (
	ReasonBuildFailed = "a página não compila, então as pendências não puderam ser lidas; veja build_errors"
	ReasonRulesBroken = "a página quebra uma regra do projeto, então as pendências não puderam ser lidas; veja build_errors"
	ReasonNoPreview   = "a prévia da página não está disponível agora, então as pendências não foram lidas"
	ReasonNoCapture   = "o editor aberto não devolveu a página renderizada, então as pendências não foram lidas"
	ReasonNoManifest  = "site.json não pôde ser lido, então as pendências não foram lidas"
	ReasonStale       = "a página mudou enquanto era lida, então as pendências não foram lidas; leia de novo"
	ReasonInvalid     = "a leitura da página chegou incompleta ou grande demais, então as pendências não foram lidas"
	reasonRouteBroken = "a rota %s não gerou um HTML legível"

	CodeBuildFailed        = "build_failed"
	CodeRulesBroken        = "rules_broken"
	CodePreviewUnavailable = "preview_unavailable"
	CodeRouteUnreadable    = "route_unreadable"
	CodeCaptureUnavailable = "capture_unavailable"
	CodeSnapshotStale      = "snapshot_stale"
	CodeSnapshotInvalid    = "snapshot_invalid"

	MaxDocumentBytes = 1536 << 10
	MaxSnapshotBytes = 6 << 20

	outlineDepth = 12
	outlineNodes = 600
)

var ErrSnapshotInvalid = errors.New("siteinspect: the snapshot does not hold exactly the page routes within the size limits")

type Previews interface {
	Preview(ctx context.Context, workspaceID, projectID string) (site_usecase.PreviewResult, error)
}

type DocumentRequest struct {
	PreviewURL string
	Routes     []string
}

type Documents interface {
	Documents(ctx context.Context, req DocumentRequest) (map[string]string, error)
}

type Snapshot map[string]string

func (s Snapshot) Documents(context.Context, DocumentRequest) (map[string]string, error) {
	return s, nil
}

type Service struct {
	Previews Previews
	Cache    InspectionCache
}

type Report struct {
	Known          bool
	Reason         string
	ReasonCode     string
	ReasonRoute    string
	Gaps           []site.Issue
	Checks         []site.Check
	OmittedChecks  int
	BuildIssues    []site_usecase.BuildIssue
	RuleIssues     []site.Issue
	Routes         []site.RouteOutline
	Forms          []string
	PreviewURL     string
	PreviewVersion string
}

func Routes(view *site_usecase.View) ([]string, bool) {
	raw, ok := view.Files[site.ManifestPath]
	if !ok {
		return nil, false
	}
	manifest, err := site.DecodeManifest(view.Project.Stack, []byte(raw.Content))
	if err != nil {
		return nil, false
	}
	return manifest.RoutePaths(), true
}

func Settings(view *site_usecase.View) site.Settings {
	if view.Settings == nil {
		return site.Settings{}
	}
	return view.Settings.Settings
}

func CheckSnapshot(documents map[string]string, routes []string) error {
	if len(documents) != len(routes) {
		return fmt.Errorf("%w: %d documents for %d routes", ErrSnapshotInvalid, len(documents), len(routes))
	}
	total := 0
	for route, document := range documents {
		if !slices.Contains(routes, route) {
			return fmt.Errorf("%w: %q is not a page route", ErrSnapshotInvalid, route)
		}
		if len(document) == 0 || len(document) > MaxDocumentBytes {
			return fmt.Errorf("%w: the document of %s has %d bytes", ErrSnapshotInvalid, route, len(document))
		}
		total += len(document)
	}
	if total > MaxSnapshotBytes {
		return fmt.Errorf("%w: the snapshot has %d bytes", ErrSnapshotInvalid, total)
	}
	return nil
}

func (s Service) Inspect(ctx context.Context, view *site_usecase.View, documents Documents) Report {
	report := baseReport(view)
	routes, ok := Routes(view)
	if !ok {
		report.Reason, report.ReasonCode = ReasonNoManifest, site.CodeManifestInvalid
		return report
	}
	preview, err := s.Previews.Preview(ctx, view.Project.WorkspaceID, view.Project.ID)
	var failed *site_usecase.BuildFailedError
	var broken *site_usecase.IssuesError
	switch {
	case errors.As(err, &failed):
		report.Reason, report.ReasonCode, report.BuildIssues = ReasonBuildFailed, CodeBuildFailed, failed.Errors
		return report
	case errors.As(err, &broken):
		report.Reason, report.ReasonCode, report.RuleIssues = ReasonRulesBroken, CodeRulesBroken, broken.Issues
		return report
	case err != nil || preview.URL == "":
		report.Reason, report.ReasonCode = ReasonNoPreview, CodePreviewUnavailable
		return report
	}
	report.PreviewURL, report.PreviewVersion = preview.URL, preview.RevisionID
	key, cacheable := inspectionKey(preview.Build, routes, Settings(view))
	if cacheable {
		if cached, ok := s.cachedInspection(key); ok {
			cached.PreviewURL, cached.PreviewVersion = preview.URL, preview.RevisionID
			return cached
		}
	}
	if documents == nil {
		report.Reason, report.ReasonCode = ReasonNoCapture, CodeCaptureUnavailable
		return report
	}
	captured, err := documents.Documents(ctx, DocumentRequest{PreviewURL: preview.URL, Routes: routes})
	if err != nil {
		report.Reason, report.ReasonCode = ReasonNoCapture, CodeCaptureUnavailable
		return report
	}
	if CheckSnapshot(captured, routes) != nil {
		report.Reason, report.ReasonCode = ReasonInvalid, CodeSnapshotInvalid
		return report
	}
	pages := make([]site.RenderedPage, 0, len(routes))
	outlines := make([]site.RouteOutline, 0, len(routes))
	for _, route := range routes {
		doc, err := sitehtml.Parse(captured[route])
		if err != nil {
			report.Reason, report.ReasonCode, report.ReasonRoute = fmt.Sprintf(reasonRouteBroken, route), CodeRouteUnreadable, route
			return report
		}
		if !site.BuiltFrom(doc, view.Project.Stack, view.Files) {
			report.Reason, report.ReasonCode, report.ReasonRoute = ReasonStale, CodeSnapshotStale, route
			return report
		}
		pages = append(pages, site.RenderedPage{Route: route, Document: doc})
		nodes := site.NodeOutline(doc, outlineDepth, outlineNodes)
		if nodes == nil {
			nodes = []site.OutlineNode{}
		}
		outlines = append(outlines, site.RouteOutline{Route: route, Rendered: true, Nodes: nodes})
	}
	checked := site.CheckPages(pages)
	structural, _ := site.Split(checked.Issues)
	report.Known, report.Routes = true, outlines
	report.Gaps = append(append(report.Gaps, structural...), checked.OpenGaps(pages, Settings(view))...)
	report.Checks, report.OmittedChecks = siterender.Condense(site.RenderedChecks(pages))
	if cacheable {
		s.keepInspection(key, report)
	}
	return report
}
