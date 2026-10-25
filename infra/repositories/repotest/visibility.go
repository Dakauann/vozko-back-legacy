package repotest

import (
	"testing"

	"gorm.io/gorm"

	"vozko/infra/database/schema"
)

func ConversationVisibilityModels() []interface{} {
	return []interface{}{
		&schema.WhatsAppCampaign{}, &schema.WhatsAppCampaignEntry{}, &schema.InstagramAccount{}, &schema.InstagramConversation{},
		&schema.FacebookPage{}, &schema.FacebookConversation{}, &schema.TelegramAccount{}, &schema.TelegramConversation{},
		&schema.WebchatWidget{}, &schema.WebchatConversation{}, &schema.MercadoLivreAccount{}, &schema.MercadoLivreConversation{},
		&schema.TikTokAccount{}, &schema.TikTokConversation{}, &schema.UnofficialWhatsAppInstance{}, &schema.UnofficialWhatsAppConversation{},
		&schema.EmailInbox{}, &schema.EmailConversation{},
		&schema.RCSSender{}, &schema.RCSConversation{}, &schema.SMSRoute{}, &schema.SMSConversation{},
		&schema.InboxAssignment{}, &schema.ConversationDepartmentRoute{}, &schema.UnofficialWhatsAppCampaign{},
	}
}

func DropForeignKeys(t testing.TB, db *gorm.DB) {
	t.Helper()
	var keys []struct {
		Owner string
		Name  string
	}
	if err := db.Raw(`SELECT conrelid::regclass::text AS owner, conname AS name FROM pg_constraint
		WHERE contype = 'f' AND connamespace = current_schema()::regnamespace`).Scan(&keys).Error; err != nil {
		t.Fatalf("list foreign keys: %v", err)
	}
	for _, key := range keys {
		if err := db.Exec(`ALTER TABLE ` + key.Owner + ` DROP CONSTRAINT "` + key.Name + `"`).Error; err != nil {
			t.Fatalf("drop %s: %v", key.Name, err)
		}
	}
}
