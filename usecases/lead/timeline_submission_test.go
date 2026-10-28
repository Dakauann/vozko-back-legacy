package lead_usecase

import (
	"context"
	"testing"

	"vozko/domain/lead"
)

func TestPageFormSubmissionsReachOnlyWhoReadsSubmissions(t *testing.T) {
	items := []lead.TimelineItem{plainItem("site_submission:s-1", lead.TimelineSiteSubmission, lead.TimelineRefSubmission, 1, "")}
	for viewer, want := range map[string]bool{callManager: true, salesMember: false} {
		f := newTimelineFixture(t, items)
		page, err := f.timeline.Page(context.Background(), timelineViewer(viewer), lead.PageQuery{LeadID: "l-1", Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if f.source.scans[0].Submissions != want || (len(page.Items) == 1) != want {
			t.Fatalf("%s: scan %v items %v", viewer, f.source.scans[0].Submissions, itemIDs(page.Items))
		}
	}
}
