package database

func leadSentimentIndexes() []ddl {
	return []ddl{
		{
			name: "idx_lead_sentiments_workspace",
			sql: `CREATE INDEX IF NOT EXISTS idx_lead_sentiments_workspace
				ON lead_sentiments (workspace_id, lead_id) INCLUDE (sentiment)`,
		},
		{
			name: "idx_lead_sentiments_analysis",
			sql: `CREATE INDEX IF NOT EXISTS idx_lead_sentiments_analysis
				ON lead_sentiments (analysis_id)`,
		},
	}
}

func LeadSentimentIndexStatements() []string {
	indexes := leadSentimentIndexes()
	out := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		out = append(out, idx.sql)
	}
	return out
}
