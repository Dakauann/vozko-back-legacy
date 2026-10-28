package lead

import "testing"

func TestAPageFormSubmissionIsShownOnlyToWhoReadsSubmissions(t *testing.T) {
	item := TimelineItem{ID: "site_submission:s-1", Kind: TimelineSiteSubmission, Ref: TimelineRef{Type: TimelineRefSubmission, ID: "s-1"}}
	if _, ok := (TimelineGrants{Submissions: true}).Visible(item); !ok {
		t.Fatal("a viewer of submissions must see it")
	}
	if _, ok := (TimelineGrants{}).Visible(item); ok {
		t.Fatal("a viewer without site_submissions:read must not see it")
	}
	item.Ref.Type = TimelineRefEvent
	if _, ok := (TimelineGrants{Submissions: true}).Visible(item); ok {
		t.Fatal("a submission must point at the submission")
	}
}
