package businessphone

func RegistrationNeeded(linked *WhatsAppBusinessPhoneNumber, wabaID string, metaStatus Status, coexistence bool) bool {
	if coexistence {
		return false
	}
	if metaStatus != StatusConnected {
		return true
	}
	return linked != nil && linked.WABAId != wabaID
}
