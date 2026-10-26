package attendance_repository

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/actor"
	"vozko/domain/attendance"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

type everySection struct {
	Summary *attendance.SummarySection
	Team    *attendance.TeamSection
	Stages  attendance.OverviewStages
	Backlog attendance.BacklogXray
	Rework  attendance.OverviewRework
	Trend   attendance.TrendResult
}

func readSections(ctx context.Context, t *testing.T, repo attendance.Repository, workspaceID string, filter attendance.OverviewFilter, now time.Time) everySection {
	t.Helper()
	var out everySection
	var err error
	if out.Summary, err = repo.ReadSummary(ctx, workspaceID, filter); err != nil {
		t.Fatalf("ReadSummary: %v", err)
	}
	out.Summary.GeneratedAt = time.Time{}
	if out.Trend, err = repo.GetTrend(ctx, workspaceID, filter, 13, time.UTC); err != nil {
		t.Fatalf("GetTrend: %v", err)
	}
	if out.Team, err = repo.ReadTeam(ctx, workspaceID, filter); err != nil {
		t.Fatalf("ReadTeam: %v", err)
	}
	if out.Stages, err = repo.ReadStages(ctx, workspaceID, filter); err != nil {
		t.Fatalf("ReadStages: %v", err)
	}
	if out.Backlog, err = repo.ReadBacklog(ctx, workspaceID, filter, now); err != nil {
		t.Fatalf("ReadBacklog: %v", err)
	}
	if out.Rework, err = repo.ReadRework(ctx, workspaceID, filter); err != nil {
		t.Fatalf("ReadRework: %v", err)
	}
	return out
}

func attendanceModels() []interface{} {
	return []interface{}{
		&schema.AgentPresenceInterval{}, &schema.Agent{}, &schema.AIAttendanceSession{}, &schema.AssignmentHistory{}, &schema.Balance{},
		&schema.ConversationEvent{}, &schema.ConversationMessage{}, &schema.ConversationOrigin{}, &schema.EntryStage{}, &schema.InboxAssignment{}, &schema.LeadMessageWindow{},
		&schema.Lead{}, &schema.Opportunity{}, &schema.OpportunityConversation{}, &schema.Pipeline{}, &schema.Stage{}, &schema.User{},
		&schema.WhatsAppBusinessPhoneNumber{}, &schema.WhatsAppCampaign{}, &schema.WhatsAppCampaignEntry{}, &schema.WhatsAppTemplateSend{},
		&schema.WorkspacePhoneAccess{}, &schema.WorkspaceDepartment{}, &schema.WorkspaceMember{}, &schema.WorkflowSchema{},
		&schema.CarrierContact{}, &schema.EmailContact{}, &schema.FacebookContact{}, &schema.InstagramContact{}, &schema.MercadoLivreContact{},
		&schema.TelegramContact{}, &schema.TikTokContact{}, &schema.UnofficialWhatsAppContact{}, &schema.WebchatVisitor{},
		&schema.EmailInbox{}, &schema.FacebookPage{}, &schema.InstagramAccount{}, &schema.MercadoLivreAccount{}, &schema.RCSSender{},
		&schema.SMSRoute{}, &schema.TelegramAccount{}, &schema.TikTokAccount{}, &schema.UnofficialWhatsAppInstance{}, &schema.WebchatWidget{},
		&schema.EmailConversation{}, &schema.FacebookConversation{}, &schema.InstagramConversation{}, &schema.MercadoLivreConversation{},
		&schema.RCSConversation{}, &schema.SMSConversation{}, &schema.TelegramConversation{}, &schema.TikTokConversation{},
		&schema.UnofficialWhatsAppConversation{}, &schema.WebchatConversation{},
	}
}

func seedAttendanceWorkspace(t *testing.T, db *gorm.DB, now time.Time) string {
	t.Helper()
	workspaceID := uuid.NewString()
	campaignID := uuid.NewString()
	agentID := uuid.NewString()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	var foreignKeys []struct {
		Owner string
		Name  string
	}
	if err := db.Raw(`SELECT conrelid::regclass::text AS owner, conname AS name FROM pg_constraint
		WHERE contype = 'f' AND connamespace = current_schema()::regnamespace`).Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("list foreign keys: %v", err)
	}
	for _, fk := range foreignKeys {
		exec(`ALTER TABLE ` + fk.Owner + ` DROP CONSTRAINT "` + fk.Name + `"`)
	}
	exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, 'Atendimento', ?, ?)`,
		campaignID, workspaceID, now.AddDate(0, -3, 0), now.AddDate(0, -3, 0))
	statuses := []string{"finished", "ongoing", "pending", "finished", "ongoing", "finished"}
	for i, status := range statuses {
		entryID := uuid.NewString()
		created := now.AddDate(0, -i, -i)
		var closedAt interface{}
		if status == "finished" {
			closedAt = created.Add(6 * time.Hour)
		}
		exec(`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id, conversation_status, closed_at, close_source, last_message_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entryID, campaignID, uuid.NewString(), status, closedAt, map[bool]string{true: "human", false: ""}[status == "finished"],
			created.Add(2*time.Hour), created, created)
		exec(`INSERT INTO inbox_assignments (id, entry_id, entry_type, assigned_user_id, workspace_id, created_at, updated_at) VALUES (?, ?, 'whatsapp', ?, ?, ?, ?)`,
			uuid.NewString(), entryID, agentID, workspaceID, created, created)
		for m, kind := range []string{"contact", string(actor.KindHuman), "contact"} {
			exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, direction, text, sender_kind, sender_id, created_at, updated_at)
				VALUES (?, ?, 'whatsapp', 'text', ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), entryID, map[bool]string{true: "inbound", false: "outbound"}[kind == "contact"], fmt.Sprintf("m%d", m), kind,
				map[bool]string{true: "", false: agentID}[kind == "contact"], created.Add(time.Duration(m+1)*time.Minute), created)
		}
	}
	return workspaceID
}

func TestASharedScopeReadsExactlyWhatSeparateReadsDo(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepared statements %v", prepared), func(t *testing.T) {
			db := repotest.IsolatedDBWith(t, "att_share", repotest.Options{PrepareStmt: prepared}, attendanceModels()...)
			now := time.Now().UTC()
			workspaceID := seedAttendanceWorkspace(t, db, now)
			repo := New(db)
			wide := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

			filters := integrationFilters()
			filters["since 2020 with ai"] = attendance.OverviewFilter{DateFrom: &wide, DateTo: &now, IncludeAI: true}
			for _, ws := range []string{emptyWorkspace, workspaceID} {
				for name, filter := range filters {
					t.Run(ws+"/"+name, func(t *testing.T) {
						separate := readSections(context.Background(), t, repo, ws, filter, now)
						var shared everySection
						err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
							shared = readSections(ctx, t, repo, ws, filter, now)
							return nil
						})
						if err != nil {
							t.Fatalf("ShareScope: %v", err)
						}
						if !reflect.DeepEqual(separate, shared) {
							t.Fatalf("a shared scope changed the answer:\nseparate %+v\nshared   %+v", separate, shared)
						}
					})
				}
			}

			all := readSections(context.Background(), t, repo, workspaceID, filters["since 2020 with ai"], now)
			if all.Summary.KPIs.Engaged == 0 || all.Summary.KPIs.Finished == 0 {
				t.Fatalf("the seeded workspace produced no engaged conversations, so the comparison proved nothing: %+v", all.Summary.KPIs)
			}
		})
	}
}
