package copilottools_test

import (
	"slices"
	"testing"

	"vozko/domain/copilot"
)

func TestTheSiteSurfaceOffersOnlyItsToolsFromTheWholeRegistry(t *testing.T) {
	view := copilot.View{Surface: copilot.SurfaceSite, ProjectID: "3b9a7c2d-5e6f-4a1b-8c9d-0e1f2a3b4c5d"}
	var offered []string
	for _, tool := range registeredTools(t) {
		if view.Offers(tool) {
			offered = append(offered, tool.Definition().Name)
		}
	}
	want := []string{"ask_user", "calculate", "load_skill", "site_capture_reference", "site_edit", "site_generate_image", "site_grep", "site_look", "site_publish", "site_read", "site_settings"}
	if !slices.Equal(offered, want) {
		t.Fatalf("the site surface offers %v, want %v (no ad tools, no panel tools)", offered, want)
	}
}
