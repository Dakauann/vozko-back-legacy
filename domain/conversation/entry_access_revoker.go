package conversation

type EntryAccessRevoker interface {
	RevokeEntryAccess(entryID string) error
}
