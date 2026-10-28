package database

func googleSheetEditIndexes() []ddl {
	return []ddl{
		{
			name: "idx_google_sheet_edit_sessions_file",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_edit_sessions_file
				ON google_sheet_edit_sessions (workspace_id, spreadsheet_id, started_at DESC, id DESC)`,
		},
		{
			name: "idx_google_sheet_edit_sessions_expiry",
			sql: `CREATE INDEX IF NOT EXISTS idx_google_sheet_edit_sessions_expiry
				ON google_sheet_edit_sessions (expires_at)
				WHERE ended_at IS NULL`,
		},
	}
}

func googleSheetEditConstraints() []ddl {
	return []ddl{
		{
			name: "ux_google_sheet_edit_sessions_open",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS ux_google_sheet_edit_sessions_open
				ON google_sheet_edit_sessions (workspace_id, spreadsheet_id)
				WHERE ended_at IS NULL`,
		},
	}
}
