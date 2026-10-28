package sitetools

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/copilot"
)

func watchOf(t *testing.T, r *rig) copilot.SurfaceWatch {
	t.Helper()
	watch, ok := NewSiteReadTool(r.deps).(copilot.SurfaceWatch)
	if !ok {
		t.Fatal("site_read watches the site surface")
	}
	return watch
}

func TestTheSnapshotNamesThePageRouteSelectionAndGapCount(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	note := watchOf(t, r).ViewSnapshot(context.Background(), r.cc)
	for _, want := range []string{`"Clínica Sorriso"`, projectID, "rota /", `h1 src/routes/index.jsx#120 "Agende"`, "site_read lê", ": 2."} {
		if !strings.Contains(note, want) {
			t.Errorf("the snapshot lacks %q: %s", want, note)
		}
	}
	if strings.Contains(note, "alt_missing") {
		t.Fatal("the snapshot carries the count, the list stays in site_read")
	}
	away := r.cc
	away.View = copilot.View{}
	if watchOf(t, r).ViewSnapshot(context.Background(), away) != "" {
		t.Fatal("no snapshot off the site surface")
	}
}

func TestTheSnapshotNeverClaimsNoGapsWhenTheyCouldNotBeRead(t *testing.T) {
	r := newRig(t)
	r.editor.pages = map[string]string{}
	note := watchOf(t, r).ViewSnapshot(context.Background(), r.cc)
	if !strings.Contains(note, "não puderam ser lidas") || strings.Contains(note, "nada impede") {
		t.Fatalf("got %s", note)
	}
}

func TestTheClosingNoteListsOnlyBlockersTheReplyLeftOut(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	watch := watchOf(t, r)
	note := watch.ClosingNote(context.Background(), r.cc, "Pronto! Falta só o link do botão Fale.")
	if !strings.Contains(note, "alt_missing") || strings.Contains(note, "link_empty") {
		t.Fatalf("got %s", note)
	}
	if note := watch.ClosingNote(context.Background(), r.cc, "Falta o texto alternativo da foto e o link do botão."); note != "" {
		t.Fatalf("a reply that names every blocker gets no note, got %s", note)
	}
	r.editor.pages = map[string]string{}
	if note := watch.ClosingNote(context.Background(), r.cc, "Pronto."); note != "" {
		t.Fatalf("an unread state stays silent, got %s", note)
	}
}
