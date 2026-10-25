package template_usecase

import (
	"context"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/whatsapp/template"
)

type singleTemplateClientMock struct {
	syncTemplatesClientMock
	fromMeta conversation.Template
}

func (m *singleTemplateClientMock) GetTemplate(context.Context, string) (*conversation.Template, error) {
	found := m.fromMeta
	return &found, nil
}

func TestSyncTemplate_RewritesOnlyWhenMetaChangedIt(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		wantUpdates int
	}{
		{name: "unchanged", status: "APPROVED", wantUpdates: 0},
		{name: "status moved", status: "PAUSED", wantUpdates: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockTemplateRepo()
			repo.templates["t1"] = &template.Template{ID: "t1", ExternalID: "ext-1", WABAId: "waba-1", Name: "welcome", Language: "pt_BR",
				Category: template.TemplateCategoryUtility, Status: template.TemplateStatusApproved, Quality: "GREEN",
				ParameterFormat: template.ParameterFormatPositional, Components: []template.TemplateComponent{{Type: "BODY", Text: "Hi"}}}
			client := &singleTemplateClientMock{fromMeta: conversation.Template{ID: "ext-1", Name: "welcome", Language: "pt_BR", Category: "UTILITY", Status: tc.status,
				Components: []conversation.TemplateComponent{{Type: "BODY", Text: "Hi"}}}}
			uc := NewSyncTemplateUseCase(repo, &syncTemplatesFactoryMock{client: client, wabaID: "waba-1"})

			got, err := uc.Execute(template.SyncTemplateInput{TemplateID: "t1"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.updated) != tc.wantUpdates {
				t.Fatalf("updates = %v, want %d", repo.updated, tc.wantUpdates)
			}
			if string(got.Status) != tc.status {
				t.Fatalf("returned status %s, want %s", got.Status, tc.status)
			}
		})
	}
}
