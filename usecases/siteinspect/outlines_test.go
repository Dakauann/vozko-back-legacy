package siteinspect

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

type countingInspector struct {
	report Report
	calls  int
	given  []Documents
}

func (c *countingInspector) Inspect(_ context.Context, _ *site_usecase.View, documents Documents) Report {
	c.calls++
	c.given = append(c.given, documents)
	return c.report
}

type memoryState struct {
	values  map[string]string
	locks   map[string]bool
	lockErr error
	ttls    map[string]time.Duration
}

func newMemoryState() *memoryState {
	return &memoryState{values: map[string]string{}, locks: map[string]bool{}, ttls: map[string]time.Duration{}}
}

func (m *memoryState) SetNX(key, _ string, ttl time.Duration) (bool, error) {
	if m.lockErr != nil {
		return false, m.lockErr
	}
	m.ttls[key] = ttl
	if m.locks[key] {
		return false, nil
	}
	m.locks[key] = true
	return true, nil
}

func (m *memoryState) GetString(key string) (string, error) { return m.values[key], nil }

func (m *memoryState) SetString(key, value string, ttl time.Duration) error {
	m.values[key], m.ttls[key] = value, ttl
	return nil
}

func inspected() Report {
	return Report{Known: true, Gaps: []site.Issue{{Code: site.CodeAltMissing, Route: "/"}}, Routes: []site.RouteOutline{{Route: "/", Rendered: true}}, Forms: []string{"contato"}, PreviewURL: "https://t.previa.test/"}
}

var snapshot = Snapshot{"/": home, "/obrigado": thanks}

func TestReadingTheOutlineNeverInspectsAndServesOnlyThisVersion(t *testing.T) {
	inspector, state := &countingInspector{report: inspected()}, newMemoryState()
	outlines := Outlines{Inspector: inspector, State: state}
	v := view()
	v.Revision.ID, v.Settings.Version = "r1", 3
	pending := outlines.Outline(context.Background(), v)
	if pending.Known || pending.ReasonCode != CodeNotInspected || inspector.calls != 0 || len(pending.Forms) != 1 || pending.Gaps == nil {
		t.Fatalf("got %+v", pending)
	}
	submitted := outlines.Submit(context.Background(), v, snapshot)
	if !submitted.Known || inspector.calls != 1 || state.ttls["site:outline:lock:p1"] != InspectSpacing {
		t.Fatalf("got %+v %v", submitted, state.ttls)
	}
	if given, ok := inspector.given[0].(Snapshot); !ok || len(given) != 2 {
		t.Fatalf("the submitted documents are what is inspected: %#v", inspector.given)
	}
	cached := outlines.Outline(context.Background(), v)
	if !cached.Known || len(cached.Gaps) != 1 || cached.PreviewURL != "https://t.previa.test/" || inspector.calls != 1 {
		t.Fatalf("got %+v", cached)
	}
	v.Revision.ID = "r2"
	if stale := outlines.Outline(context.Background(), v); stale.Known || stale.ReasonCode != CodeNotInspected {
		t.Fatalf("an outline of an older version is never served: %+v", stale)
	}
	v.Revision.ID, v.Settings.Version = "r1", 4
	if stale := outlines.Outline(context.Background(), v); stale.Known {
		t.Fatalf("settings change the gaps: %+v", stale)
	}
}

func TestSnapshotsAreThrottledPerProject(t *testing.T) {
	inspector, state := &countingInspector{report: inspected()}, newMemoryState()
	outlines := Outlines{Inspector: inspector, State: state}
	v := view()
	v.Revision.ID = "r1"
	outlines.Submit(context.Background(), v, snapshot)
	again := outlines.Submit(context.Background(), v, snapshot)
	if inspector.calls != 1 || !again.Known {
		t.Fatalf("a throttled editor gets the stored outline: %d %+v", inspector.calls, again)
	}
	v.Revision.ID = "r2"
	throttled := outlines.Submit(context.Background(), v, snapshot)
	if inspector.calls != 1 || throttled.Known || throttled.ReasonCode != CodeInspectionThrottled {
		t.Fatalf("got %+v", throttled)
	}
	state.locks = map[string]bool{}
	state.lockErr = errors.New("redis down")
	if down := outlines.Submit(context.Background(), v, snapshot); inspector.calls != 1 || down.ReasonCode != CodeInspectionThrottled {
		t.Fatalf("an unreachable throttle never inspects: %+v", down)
	}
}

func TestPassingFailuresAreNotStored(t *testing.T) {
	inspector, state := &countingInspector{}, newMemoryState()
	outlines := Outlines{Inspector: inspector, State: state}
	v := view()
	for _, code := range []string{CodePreviewUnavailable, CodeSnapshotStale, CodeSnapshotInvalid, CodeCaptureUnavailable, CodeRouteUnreadable} {
		inspector.report = Report{ReasonCode: code}
		state.locks = map[string]bool{}
		outlines.Submit(context.Background(), v, snapshot)
		if len(state.values) != 0 {
			t.Fatalf("%s stays out of the store: %v", code, state.values)
		}
	}
	inspector.report = Report{ReasonCode: CodeBuildFailed, BuildIssues: []site_usecase.BuildIssue{{Path: "src/App.jsx", Line: 3, Message: "x"}}}
	state.locks = map[string]bool{}
	outlines.Submit(context.Background(), v, snapshot)
	if got := outlines.Outline(context.Background(), v); got.ReasonCode != CodeBuildFailed || len(got.BuildIssues) != 1 {
		t.Fatalf("a build failure is the outline of this version: %+v", got)
	}
}
