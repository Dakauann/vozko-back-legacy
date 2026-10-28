package lead

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/lead"
	"vozko/infra/database/schema"
)

func TestAPageFormSubmissionReachesTheLeadTimelineAgainstPostgres(t *testing.T) {
	w := newTimelineWorld(t)
	models := slices.Concat([]any{&schema.SiteSubmission{}, &schema.MercadoLivreAccount{}, &schema.MercadoLivreContact{}, &schema.MercadoLivreConversation{}},
		schema.TikTokModels(), schema.EmailInboxModels(), schema.CarrierMessagingModels(), schema.EmailSendingModels())
	if err := w.db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	project, leadID := uuid.NewString(), w.maria.ID
	row := schema.SiteSubmission{
		ID: uuid.NewString(), WorkspaceID: w.ws, ProjectID: project, DeploymentID: uuid.NewString(), FormKey: "k", FormID: "contato", Route: "/oferta",
		Fields: []byte(`{"lead.name":"Maria"}`), UTM: []byte(`{"source":"meta"}`), Consent: []byte(`{}`), LeadID: &leadID, IdempotencyKey: "idem-12345678",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := w.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	items := w.scan(t, lead.TimelineScan{Limit: 50, Submissions: true})
	var found *lead.TimelineItem
	for i := range items {
		if items[i].Kind == lead.TimelineSiteSubmission {
			found = &items[i]
		}
	}
	if found == nil || found.Ref.ID != row.ID || found.Summary.ProjectID != project || found.Summary.Title != "contato" || found.Summary.Source != "meta" || found.Summary.Text != "/oferta" {
		t.Fatalf("submission item %+v in %v", found, timelineIDs(items))
	}
	for _, item := range w.scan(t, lead.TimelineScan{Limit: 50}) {
		if item.Kind == lead.TimelineSiteSubmission {
			t.Fatal("submissions read without the grant")
		}
	}
}
