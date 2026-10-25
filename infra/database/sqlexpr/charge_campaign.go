package sqlexpr

const NoChargeCampaign = "00000000-0000-0000-0000-000000000000"

const canonicalUUIDPrefix = "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"

func ChargeOwnerKey(referenceColumn string) string {
	bare := "regexp_replace(" + referenceColumn + ", '^refund:', '')"
	return "CASE WHEN " + bare + " LIKE 'waba:%' THEN " + bare + " ELSE split_part(" + bare + ", ':', 1) END"
}

func canonicalUUID(text string) string {
	return "CASE WHEN length(" + text + ") = 36 AND " + text + " ~ '" + canonicalUUIDPrefix + "' THEN (" + text + ")::uuid END"
}

func ChargeCampaignOf(referenceColumn string) string {
	return `(SELECT COALESCE(
			(SELECT c.id FROM whatsapp_campaigns c WHERE c.id = owner_ids.plain),
			(SELECT e.campaign_id FROM whatsapp_campaign_entries e WHERE e.id = owner_ids.plain),
			(SELECT s.campaign_id FROM whatsapp_template_sends s WHERE s.id = owner_ids.waba),
			'` + NoChargeCampaign + `'::uuid)
		FROM (
			SELECT ` + canonicalUUID("refs.owner_key") + ` AS plain,
				CASE WHEN refs.owner_key LIKE 'waba:%' THEN ` + canonicalUUID("substr(refs.owner_key, 6)") + ` END AS waba
			FROM (SELECT ` + ChargeOwnerKey(referenceColumn) + ` AS owner_key) refs
		) owner_ids)`
}
