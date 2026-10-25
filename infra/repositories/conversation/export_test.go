package conversation_repository

import (
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func WorkspaceWindowFilterIDs(db *gorm.DB, workspaceID string, entryType shared.EntryType, open bool) ([]string, error) {
	queries, ok := workspaceInboxQueries(conversation.SearchEntriesInput{
		WorkspaceID: workspaceID, EntryType: entryType, WindowOpen: &open,
	}, 50, 0)
	if !ok {
		return nil, nil
	}
	return pageEntryIDs(db, queries)
}

func ContainerWindowFilterIDs(db *gorm.DB, entryType shared.EntryType, containerID string, open bool) ([]string, error) {
	ch, ok := channelQueryFor(entryType)
	if !ok {
		return nil, nil
	}
	queries := containerInboxQueries(conversation.SearchEntriesInput{
		CampaignID: containerID, EntryType: entryType, WindowOpen: &open,
	}, ch, nil, 50, 0)
	return pageEntryIDs(db, queries)
}

func pageEntryIDs(db *gorm.DB, queries countedPage) ([]string, error) {
	var rows []struct {
		EntryID string `gorm:"column:entry_id"`
	}
	if err := db.Raw(queries.pageSQL, queries.pageArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.EntryID
	}
	return ids, nil
}

func InboxDepartmentOf(db *gorm.DB, entryType shared.EntryType, workspaceID, entryID string) (string, error) {
	for _, src := range entrySources {
		if src.EntryType != entryType {
			continue
		}
		var rows []struct {
			DepartmentID *string `gorm:"column:department_id"`
		}
		query := "SELECT (" + src.department() + ")::text AS department_id FROM " + src.From + " " + src.WorkspaceJoin + " WHERE " + src.EntryID + " = ?"
		if err := db.Raw(query, workspaceID, entryID).Scan(&rows).Error; err != nil {
			return "", err
		}
		if len(rows) == 0 || rows[0].DepartmentID == nil {
			return "", nil
		}
		return *rows[0].DepartmentID, nil
	}
	return "", nil
}
