package database

func googleSheetRemovalIndexes() []ddl {
	return []ddl{
		{
			name: "idx_google_sheet_unique_keys_row",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_unique_keys_row
				ON google_sheet_unique_keys (sheet_id, row_id)`,
		},
		{
			name: "idx_google_sheet_row_removals_sheet",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_row_removals_sheet
				ON google_sheet_row_removals (sheet_id, removed_at DESC, id DESC)`,
		},
		{
			name: "idx_google_sheet_row_removals_lead",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_row_removals_lead
				ON google_sheet_row_removals (workspace_id, lead_id)
				WHERE lead_id IS NOT NULL`,
		},
		{
			name: "idx_google_sheet_row_removals_entry",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_row_removals_entry
				ON google_sheet_row_removals (entry_type, entry_id)
				WHERE entry_id <> ''`,
		},
	}
}

func googleSheetRemovalConstraints() []ddl {
	return []ddl{
		{
			name: "ux_google_sheet_row_removals_row",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS ux_google_sheet_row_removals_row
				ON google_sheet_row_removals (row_id)`,
		},
	}
}
