package googlesheet_repository

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
)

func TestCampaignsUsingAFileAreTheRunningOnesWhoseContactsCarryIt(t *testing.T) {
	db := integrationDB(t)
	db.Config.DisableForeignKeyConstraintWhenMigrating = true
	if err := db.AutoMigrate(&schema.WhatsAppCampaign{}, &schema.WhatsAppCampaignEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	workspace := uuid.NewString()
	campaignWith := func(name, status, workspaceID string, metadata schema.LeadMetadata) {
		t.Helper()
		id := uuid.NewString()
		if err := db.Exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, now(), now())`, id, workspaceID, name, status).Error; err != nil {
			t.Fatalf("campaign: %v", err)
		}
		entry := schema.WhatsAppCampaignEntry{CampaignID: id, LeadID: uuid.NewString(), Metadata: metadata}
		if err := db.Omit("Lead").Create(&entry).Error; err != nil {
			t.Fatalf("entry: %v", err)
		}
	}
	campaignWith("Cobrança Centro", "RUNNING", workspace, schema.LeadMetadata{"planilha_franquia": "sp-centro", "matricula": "1"})
	campaignWith("Cobrança Centro pausada", "PAUSED", workspace, schema.LeadMetadata{"planilha_franquia": "sp-centro"})
	campaignWith("Cobrança antiga", "COMPLETED", workspace, schema.LeadMetadata{"planilha_franquia": "sp-centro"})
	campaignWith("Cobrança Norte", "RUNNING", workspace, schema.LeadMetadata{"planilha_franquia": "sp-norte"})
	campaignWith("Outro workspace", "RUNNING", uuid.NewString(), schema.LeadMetadata{"planilha_franquia": "sp-centro"})
	got, err := NewCampaignUsage(db).CampaignsUsing(context.Background(), workspace, "sp-centro")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"Cobrança Centro", "Cobrança Centro pausada"}) {
		t.Fatalf("campaigns = %v", got)
	}
}
