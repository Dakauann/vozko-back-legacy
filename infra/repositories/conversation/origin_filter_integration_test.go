package conversation_repository

import (
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

func seedOriginWorld(t *testing.T) inboxWorld {
	t.Helper()
	w := seedInboxWorld(t)
	exec := execOrFail(t, w.db)
	for name, id := range w.byName {
		if err := recordOriginOf(w.db, id, string(shared.EntryTypeTelegram)); err != nil {
			t.Fatalf("record origin of %s: %v", name, err)
		}
	}
	exec(`UPDATE conversation_origins SET origin = 'campaign' WHERE entry_id = ?`, w.byName["customer-at-12"])
	exec(`DELETE FROM conversation_origins WHERE entry_id = ?`, w.byName["customer-at-9.5"])
	return w
}

func TestTheOriginFilterComposesWithEveryInboxScopeAgainstPostgres(t *testing.T) {
	w := seedOriginWorld(t)
	repo := &repository{db: w.db}
	exec := execOrFail(t, w.db)
	if err := w.db.AutoMigrate(&schema.ConversationDepartmentRoute{}); err != nil {
		t.Fatal(err)
	}
	sales := uuid.NewString()
	exec(`UPDATE telegram_accounts SET department_id = ? WHERE id = ?`, sales, w.account)
	exec(`INSERT INTO conversation_department_routes (entry_id, entry_type, department_id, routed_at) VALUES (?, 'telegram', ?, now())`, w.byName["customer-at-10"], uuid.NewString())
	exec(`INSERT INTO inbox_assignments (id, workspace_id, entry_id, entry_type, assigned_user_id) VALUES (?, ?, ?, 'telegram', ?)`,
		uuid.NewString(), w.workspace, w.byName["team-at-11"], w.bia)

	paths := map[string]conversation.SearchEntriesInput{
		"workspace": {WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram},
		"container": {CampaignID: w.account, EntryType: shared.EntryTypeTelegram},
	}
	cases := []struct {
		name   string
		adjust func(*conversation.SearchEntriesInput)
		want   []string
	}{
		{"receptive keeps the conversations the contact opened", func(in *conversation.SearchEntriesInput) {
			in.Origin = conversation.OriginReceptive
		}, []string{"customer-at-10"}},
		{"campaign keeps the campaign openings", func(in *conversation.SearchEntriesInput) {
			in.Origin = conversation.OriginCampaign
		}, []string{"customer-at-12"}},
		{"outreach keeps what the team opened", func(in *conversation.SearchEntriesInput) {
			in.Origin = conversation.OriginOutreach
		}, []string{"team-at-11", "pinned-by-ana"}},
		{"unknown never matches a conversation without a recorded origin", func(in *conversation.SearchEntriesInput) {
			in.Origin = conversation.OriginUnknown
		}, nil},
		{"finished conversations keep their origin", func(in *conversation.SearchEntriesInput) {
			in.Origin, in.ConversationStatus = conversation.OriginReceptive, conversation.ConversationStatusFinished
		}, []string{"finished-recently", "finished-long-ago-waiting"}},
		{"assignment visibility still hides other people's conversations", func(in *conversation.SearchEntriesInput) {
			in.Origin, in.AssignedUserID = conversation.OriginOutreach, w.ana
		}, []string{"pinned-by-ana"}},
		{"department scope still hides routed conversations", func(in *conversation.SearchEntriesInput) {
			in.Origin, in.DepartmentIDs, in.RestrictDepartments = conversation.OriginReceptive, []string{sales}, true
		}, nil},
		{"department scope keeps its own conversations of that origin", func(in *conversation.SearchEntriesInput) {
			in.Origin, in.DepartmentIDs, in.RestrictDepartments = conversation.OriginOutreach, []string{sales}, true
		}, []string{"team-at-11", "pinned-by-ana"}},
		{"labels and origin both apply", func(in *conversation.SearchEntriesInput) {
			in.Origin, in.LabelID = conversation.OriginReceptive, w.label
		}, []string{"customer-at-10"}},
	}
	for path, base := range paths {
		for _, c := range cases {
			t.Run(path+"/"+c.name, func(t *testing.T) {
				in := base
				in.Page, in.PageSize = 1, 50
				c.adjust(&in)
				entries, total, err := repo.SearchEntriesWithMessages(in)
				if err != nil {
					t.Fatal(err)
				}
				got := w.namesOf(entries)
				if !reflect.DeepEqual(sortedNames(got), sortedNames(c.want)) || total != int64(len(c.want)) {
					t.Fatalf("got %v (total %d), want %v", got, total, c.want)
				}
				for _, e := range entries {
					if c.want != nil && e.Origin != in.Origin {
						t.Fatalf("%s listed with origin %q under the %q filter", w.names[e.EntryID], e.Origin, in.Origin)
					}
				}
			})
		}
	}
}

func TestListedConversationsCarryTheirRecordedOriginAgainstPostgres(t *testing.T) {
	w := seedOriginWorld(t)
	repo := &repository{db: w.db}
	entries, _, err := repo.SearchEntriesWithMessages(conversation.SearchEntriesInput{
		WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]conversation.Origin{
		"customer-at-10":  conversation.OriginReceptive,
		"team-at-11":      conversation.OriginOutreach,
		"customer-at-12":  conversation.OriginCampaign,
		"customer-at-9.5": "",
		"pinned-by-ana":   conversation.OriginOutreach,
	}
	if len(entries) != len(want) {
		t.Fatalf("listed %v, want %d conversations", w.namesOf(entries), len(want))
	}
	for _, e := range entries {
		if got := e.Origin; got != want[w.names[e.EntryID]] {
			t.Errorf("%s origin = %q, want %q", w.names[e.EntryID], got, want[w.names[e.EntryID]])
		}
	}
	single, err := repo.GetEntryLastMessage(w.byName["customer-at-12"], shared.EntryTypeTelegram)
	if err != nil || single == nil {
		t.Fatalf("single entry: %v %v", single, err)
	}
	if single.Origin != conversation.OriginCampaign {
		t.Fatalf("the realtime entry origin = %q, want campaign", single.Origin)
	}
}

func TestOriginFilteredStageCountsMatchThePagedTotalsAgainstPostgres(t *testing.T) {
	w := seedOriginWorld(t)
	repo := &repository{db: w.db}
	exec := execOrFail(t, w.db)
	exec(`CREATE TABLE entry_stages (id text PRIMARY KEY, stage_id uuid NOT NULL, entry_id uuid NOT NULL, entry_type varchar(20) NOT NULL,
		workspace_id uuid NOT NULL, created_at timestamptz, deleted_at timestamptz)`)
	stageA, stageB := uuid.NewString(), uuid.NewString()
	for name, stageID := range map[string]string{"customer-at-10": stageA, "team-at-11": stageA, "customer-at-12": stageB, "customer-at-9.5": stageB} {
		exec(`INSERT INTO entry_stages (id, stage_id, entry_id, entry_type, workspace_id, created_at) VALUES (?, ?, ?, 'telegram', ?, now())`,
			uuid.NewString(), stageID, w.byName[name], w.workspace)
	}
	stageIDs := []string{stageA, stageB, unstagedStageID}
	for path, base := range map[string]conversation.SearchEntriesInput{
		"workspace": {WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace},
		"container": {WorkspaceID: w.workspace, CampaignID: w.account, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace},
	} {
		for _, origin := range conversation.Origins() {
			t.Run(path+"/"+string(origin), func(t *testing.T) {
				in := base
				in.Origin = origin
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
					if grouped[id] != total {
						t.Fatalf("stage %s: grouped %d, paged total %d", id, grouped[id], total)
					}
				}
			})
		}
	}
	receptive, err := repo.CountEntriesByStages(conversation.SearchEntriesInput{
		WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: w.workspace, Origin: conversation.OriginReceptive,
	}, stageIDs)
	if err != nil {
		t.Fatal(err)
	}
	if receptive[stageA] != 1 || receptive[stageB] != 0 || receptive[unstagedStageID] != 0 {
		t.Fatalf("receptive stage counts = %v", receptive)
	}
}

func sortedNames(names []string) []string {
	out := append([]string{}, names...)
	sort.Strings(out)
	return out
}
