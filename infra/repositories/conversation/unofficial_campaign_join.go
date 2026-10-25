package conversation_repository

func UnofficialCampaignJoin(conv, camp string) string {
	return "LEFT JOIN unofficial_whatsapp_campaigns " + camp + " ON " + unofficialCampaignMatch(conv, camp)
}

func UnofficialDepartment(conv, instance string) string {
	return "COALESCE((SELECT camp_dept.department_id FROM unofficial_whatsapp_campaigns camp_dept WHERE " +
		unofficialCampaignMatch(conv, "camp_dept") + "), " + instance + ".department_id)"
}

func unofficialCampaignMatch(conv, camp string) string {
	return camp + ".id = NULLIF(" + conv + ".campaign_id, '')::uuid AND " + camp + ".deleted_at IS NULL"
}
