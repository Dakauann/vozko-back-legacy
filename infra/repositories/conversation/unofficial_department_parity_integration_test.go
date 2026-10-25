package conversation_repository_test

import (
	"testing"

	"github.com/google/uuid"

	"vozko/domain/shared"
	"vozko/infra/database/schema"
	conversation_repository "vozko/infra/repositories/conversation"
	"vozko/infra/repositories/conversationroute"
	"vozko/infra/repositories/repotest"
	unofficial_whatsapp_repository "vozko/infra/repositories/unofficial_whatsapp"
	conversation_usecase "vozko/usecases/conversation"
)

func TestTheUnofficialInboxAndTheResolverAgreeOnTheDepartment(t *testing.T) {
	repotest.WithPII(t)
	db := repotest.IsolatedDB(t, "uw_department_parity",
		&schema.UnofficialWhatsAppInstance{}, &schema.UnofficialWhatsAppConversation{},
		&schema.UnofficialWhatsAppCampaign{}, &schema.ConversationDepartmentRoute{})
	workspace := uuid.NewString()
	routes := conversationroute.New(db)
	resolver := conversation_usecase.NewCampaignWorkspaceResolver(nil, nil, routes)
	resolver.(interface {
		SetEntryOwnerResolver(shared.EntryType, conversation_usecase.EntryOwnerResolver)
	}).SetEntryOwnerResolver(shared.EntryTypeUnofficialWhatsApp, unofficial_whatsapp_repository.NewConversationRepository(db))

	create := func(record interface{}) {
		t.Helper()
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	number := func(departmentID *string) string {
		record := &schema.UnofficialWhatsAppInstance{WorkspaceID: workspace, DepartmentID: departmentID, ServerID: uuid.NewString(),
			ProviderInstanceID: uuid.NewString()[:16], DeliveryTokenHash: uuid.NewString(), Status: "CONNECTED"}
		create(record)
		return record.ID
	}
	campaign := func(departmentID *string, deleted bool) string {
		record := &schema.UnofficialWhatsAppCampaign{ID: uuid.NewString(), WorkspaceID: workspace, DepartmentID: departmentID, Name: "campanha"}
		create(record)
		if deleted {
			if err := db.Delete(record).Error; err != nil {
				t.Fatal(err)
			}
		}
		return record.ID
	}
	conversation := func(instanceID, campaignID string) string {
		record := &schema.UnofficialWhatsAppConversation{ID: uuid.NewString(), WorkspaceID: workspace, InstanceID: instanceID,
			ContactID: uuid.NewString(), ChatID: uuid.NewString()[:20] + "@s.whatsapp.net", CampaignID: campaignID}
		create(record)
		return record.ID
	}

	numberDept, campaignDept, routedDept := uuid.NewString(), uuid.NewString(), uuid.NewString()
	routed := conversation(number(&numberDept), campaign(&campaignDept, false))
	if err := routes.RouteToDepartment(routed, string(shared.EntryTypeUnofficialWhatsApp), routedDept); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		conversation string
		want         string
	}{
		{"a routed department wins over the campaign and the number", routed, routedDept},
		{"the campaign department wins when it differs from the number's", conversation(number(&numberDept), campaign(&campaignDept, false)), campaignDept},
		{"a campaign without a department falls back to the number's", conversation(number(&numberDept), campaign(nil, false)), numberDept},
		{"a conversation outside any campaign uses the number's", conversation(number(&numberDept), ""), numberDept},
		{"a deleted campaign falls back to the number's", conversation(number(&numberDept), campaign(&campaignDept, true)), numberDept},
		{"no department anywhere is none", conversation(number(nil), ""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fromGo, err := resolver.GetEntryDepartmentID(tc.conversation, string(shared.EntryTypeUnofficialWhatsApp))
			if err != nil {
				t.Fatalf("resolver: %v", err)
			}
			fromSQL, err := conversation_repository.InboxDepartmentOf(db, shared.EntryTypeUnofficialWhatsApp, workspace, tc.conversation)
			if err != nil {
				t.Fatalf("inbox: %v", err)
			}
			if fromGo != tc.want || fromSQL != tc.want {
				t.Fatalf("resolver = %q, inbox = %q, want %q", fromGo, fromSQL, tc.want)
			}
		})
	}
}
