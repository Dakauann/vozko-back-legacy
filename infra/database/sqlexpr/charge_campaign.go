package sqlexpr

const NoChargeCampaign = "00000000-0000-0000-0000-000000000000"

func ChargeOwnerKey(referenceColumn string) string {
	bare := "regexp_replace(" + referenceColumn + ", '^refund:', '')"
	return "CASE WHEN " + bare + " LIKE 'waba:%' THEN " + bare + " ELSE split_part(" + bare + ", ':', 1) END"
}

func ChargeCampaignOf(referenceColumn string) string {
	return `(SELECT COALESCE(
			(SELECT c.id FROM whatsapp_campaigns c WHERE c.id = owner_ids.plain),
			(SELECT e.campaign_id FROM whatsapp_campaign_entries e WHERE e.id = owner_ids.plain),
			(SELECT s.campaign_id FROM whatsapp_template_sends s WHERE s.id = owner_ids.waba),
			'` + NoChargeCampaign + `'::uuid)
		FROM (
			SELECT ` + CanonicalUUID("refs.owner_key") + ` AS plain,
				CASE WHEN refs.owner_key LIKE 'waba:%' THEN ` + CanonicalUUID("substr(refs.owner_key, 6)") + ` END AS waba
			FROM (SELECT ` + ChargeOwnerKey(referenceColumn) + ` AS owner_key) refs
		) owner_ids)`
}
