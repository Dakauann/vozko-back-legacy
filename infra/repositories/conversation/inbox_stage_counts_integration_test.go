package conversation_repository

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

func TestGroupedStageCountsMatchThePerStageTotalsAgainstPostgres(t *testing.T) {
	w := seedInboxWorld(t)
	repo := &repository{db: w.db}
	exec := execOrFail(t, w.db)
	if err := w.db.AutoMigrate(&schema.ConversationDepartmentRoute{}); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE TABLE entry_stages (id text PRIMARY KEY, stage_id uuid NOT NULL, entry_id uuid NOT NULL, entry_type varchar(20) NOT NULL,
		workspace_id uuid NOT NULL, created_at timestamptz, deleted_at timestamptz)`)
	exec(`ALTER TABLE inbox_assignments ALTER COLUMN assigned_user_id DROP NOT NULL`)

	sales, support := uuid.NewString(), uuid.NewString()
	exec(`UPDATE telegram_accounts SET department_id = ? WHERE id = ?`, sales, w.account)
	exec(`INSERT INTO conversation_department_routes (entry_id, entry_type, department_id, routed_at) VALUES (?, 'telegram', ?, now())`, w.byName["team-at-11"], support)

	stranger, otherWorkspace := uuid.NewString(), uuid.NewString()
	seed := &assignmentSeed{exec: exec}
	seed.assign(w.workspace, w.byName["team-at-11"], &w.ana)
	seed.assign(w.workspace, w.byName["customer-at-12"], &w.ana)
	seed.assign(w.workspace, w.byName["customer-at-12"], &w.bia)
	seed.assign(w.workspace, w.byName["pinned-by-ana"], nil)
	seed.assign(w.workspace, w.byName["customer-at-10"], &w.bia)

	stageA, stageB, stageDeleted, stageForeign, stageEmpty := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	stage := func(stageID, name, workspace string, deleted bool) {
		t.Helper()
		deletedAt := "NULL"
		if deleted {
			deletedAt = "now()"
		}
		exec(`INSERT INTO entry_stages (id, stage_id, entry_id, entry_type, workspace_id, created_at, deleted_at) VALUES (?, ?, ?, 'telegram', ?, now(), `+deletedAt+`)`,
			uuid.NewString(), stageID, w.byName[name], workspace)
	}
	stage(stageA, "customer-at-10", w.workspace, false)
	stage(stageA, "customer-at-10", w.workspace, false)
	stage(stageA, "team-at-11", w.workspace, false)
	stage(stageA, "finished-recently", w.workspace, false)
	stage(stageB, "customer-at-10", w.workspace, false)
	stage(stageB, "pinned-by-ana", w.workspace, false)
	stage(stageB, "customer-at-12", w.workspace, true)
	stage(stageDeleted, "customer-at-12", w.workspace, true)
	stage(stageForeign, "customer-at-9.5", otherWorkspace, false)

	stageIDs := []string{stageB, unstagedStageID, stageA, stageEmpty, stageDeleted, stageForeign, stageA}
	all := w.ids("customer-at-10", "team-at-11", "customer-at-9.5", "customer-at-12", "pinned-by-ana", "finished-long-ago-waiting", "finished-recently")
	paths := map[string]conversation.SearchEntriesInput{
		"workspace": {WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace},
		"container": {WorkspaceID: w.workspace, CampaignID: w.account, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace},
		"entry ids": {EntryIDs: all, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace},
	}
	views := map[string]func(*conversation.SearchEntriesInput){
		"everyone":            func(*conversation.SearchEntriesInput) {},
		"ana unassigned+mine": func(in *conversation.SearchEntriesInput) { in.AssignedUserID = w.ana },
		"bia unassigned+mine": func(in *conversation.SearchEntriesInput) { in.AssignedUserID = w.bia },
		"stranger":            func(in *conversation.SearchEntriesInput) { in.AssignedUserID = stranger },
		"sales department": func(in *conversation.SearchEntriesInput) {
			in.DepartmentIDs, in.RestrictDepartments = []string{sales}, true
		},
		"support department": func(in *conversation.SearchEntriesInput) {
			in.DepartmentIDs, in.RestrictDepartments = []string{support}, true
		},
		"no department, assignee": func(in *conversation.SearchEntriesInput) {
			in.RestrictDepartments, in.AssigneeOverrideUserID = true, w.bia
		},
		"finished": func(in *conversation.SearchEntriesInput) {
			in.ConversationStatus = conversation.ConversationStatusFinished
		},
		"nothing matches":    func(in *conversation.SearchEntriesInput) { in.LabelID = uuid.NewString() },
		"no stage workspace": func(in *conversation.SearchEntriesInput) { in.StageWorkspaceID = "" },
	}

	nonZero := 0
	for pathName, base := range paths {
		for viewName, adjust := range views {
			t.Run(pathName+"/"+viewName, func(t *testing.T) {
				in := base
				adjust(&in)
				grouped, err := repo.CountEntriesByStages(in, stageIDs)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range stageIDs {
					per := in
					per.StageIDs, per.Page, per.PageSize = []string{id}, 1, 1
					_, total, err := repo.SearchEntriesWithMessages(per)
					if err != nil {
						t.Fatal(err)
					}
					got, present := grouped[id]
					if !present || got != total {
						t.Fatalf("stage %s: grouped %d (reported %v), per-stage %d", id, got, present, total)
					}
					if total > 0 {
						nonZero++
					}
				}
			})
		}
	}
	if nonZero == 0 {
		t.Fatal("every count was zero, so the comparison proved nothing")
	}

	everyone, err := repo.CountEntriesByStages(paths["workspace"], stageIDs)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{stageA: 2, stageB: 2, unstagedStageID: 2, stageEmpty: 0, stageDeleted: 0, stageForeign: 0}
	if fmt.Sprint(everyone) != fmt.Sprint(want) {
		t.Fatalf("workspace counts = %v, want %v", everyone, want)
	}
}
