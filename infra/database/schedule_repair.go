package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const consumeUsedSchedulesSQL = `
	UPDATE %[1]s AS c
	   SET scheduled_start = '0001-01-01 00:00:00+00'%[3]s
	 WHERE c.scheduled_start > '0001-01-01 00:00:00+00'
	   AND c.scheduled_start <= now()
	   AND (c.status <> 'STOPPED'
	        OR EXISTS (SELECT 1 FROM %[2]s e
	                    WHERE e.campaign_id = c.id AND e.status <> 'PENDING'))`

type scheduledCampaignTable struct {
	campaigns  string
	entries    string
	companions []string
}

var scheduledCampaignTables = []scheduledCampaignTable{
	{campaigns: "whatsapp_campaigns", entries: "whatsapp_campaign_entries", companions: []string{"scheduled_by", "schedule_fingerprint"}},
	{campaigns: "unofficial_whatsapp_campaigns", entries: "unofficial_whatsapp_campaign_entries"},
}

func consumeUsedSchedules(tx *gorm.DB) error {
	for _, table := range scheduledCampaignTables {
		if !tx.Migrator().HasColumn(table.campaigns, "scheduled_start") || !tx.Migrator().HasTable(table.entries) {
			continue
		}
		if err := tx.Exec(fmt.Sprintf(consumeUsedSchedulesSQL, table.campaigns, table.entries, companionResets(tx, table))).Error; err != nil {
			return fmt.Errorf("%s: %w", table.campaigns, err)
		}
	}
	return nil
}

func companionResets(tx *gorm.DB, table scheduledCampaignTable) string {
	var resets strings.Builder
	for _, column := range table.companions {
		if tx.Migrator().HasColumn(table.campaigns, column) {
			resets.WriteString(", " + column + " = ''")
		}
	}
	return resets.String()
}
