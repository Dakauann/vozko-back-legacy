package whatsapp_outreach

type OpenEntryInput struct {
	WorkspaceID     string
	BusinessPhoneID string
	PhoneNumber     string
	Name            string
}

type OpenedEntry struct {
	EntryID   string
	EntryType string
	LeadID    string
	Existed   bool
}
