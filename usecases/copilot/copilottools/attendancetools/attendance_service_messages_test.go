package attendancetools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"vozko/domain/attendance"
	"vozko/domain/copilot"
	"vozko/usecases/copilot/copilottools/tooltest"
)

type fakeServiceMessages struct {
	queries []attendance.ServiceMessageQuery
	err     error
}

func (f *fakeServiceMessages) AvailableSections(context.Context, string) []attendance.Section {
	return attendance.AvailableSections(f.err == nil)
}

func (f *fakeServiceMessages) ServiceMessages(_ context.Context, _ string, q attendance.ServiceMessageQuery) (*attendance.ServiceMessagesSection, error) {
	f.queries = append(f.queries, q)
	if f.err != nil {
		return nil, f.err
	}
	out := attendance.BuildServiceMessages(q, []attendance.ServiceMessageCount{
		{Bucket: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Messages: 120},
	})
	out.PriceAt(20_000, 5)
	return &out, nil
}

func serviceMessageDeps(messages *fakeServiceMessages) AttendanceDeps {
	deps := testDeps(&fakeSections{})
	deps.ServiceMessages = messages
	return deps
}

func TestEloCountsTheServiceMessagesOfAPayingWorkspaceOnScreen(t *testing.T) {
	messages := &fakeServiceMessages{}
	cc := tooltest.OwnerOn(screen)
	res := NewAttendanceServiceMessagesTool(serviceMessageDeps(messages)).Execute(context.Background(), cc, nil)
	if res.Status != copilot.StatusOK {
		t.Fatalf("status = %v (%s)", res.Status, res.Message)
	}
	got := messages.queries[0].Filter
	if day(got) != "2026-09-01..2026-09-07" || got.DepartmentID != tooltest.KnownDepartment || got.Channel != "whatsapp" {
		t.Fatalf("filter = %s dept=%q channel=%q", day(got), got.DepartmentID, got.Channel)
	}
	data := tooltest.DataOf(res)
	digest := data["service_messages"].(attendance.ServiceMessagesDigest)
	if digest.Total == nil || *digest.Total != 120 || digest.Spend == nil {
		t.Fatalf("digest = %+v", digest)
	}
	preview := data["dataset"].(copilot.DatasetPreview)
	stored, err := cc.Datasets.Get(preview.DatasetID)
	if err != nil || len(stored.Rows) != 7 {
		t.Fatalf("stored = %v err %v, want one row per day", stored, err)
	}
}

func TestEloTellsAWorkspaceThatDoesNotPayThatTheNumberIsNotAvailable(t *testing.T) {
	messages := &fakeServiceMessages{err: attendance.ErrServiceMessagesNotOnPlan}
	res := NewAttendanceServiceMessagesTool(serviceMessageDeps(messages)).Execute(context.Background(), tooltest.OwnerOn(screen), nil)
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "plano") {
		t.Fatalf("res = %+v, want the plan named as the reason", res)
	}
	if res.Data != nil {
		t.Fatalf("a refused answer carries data: %+v", res.Data)
	}
}

func TestEloReturnsNothingWhenThePriceCannotBeResolved(t *testing.T) {
	messages := &fakeServiceMessages{err: fmt.Errorf("%w: plan lookup failed", attendance.ErrServiceMessagePriceUnavailable)}
	res := NewAttendanceServiceMessagesTool(serviceMessageDeps(messages)).Execute(context.Background(), tooltest.OwnerOn(screen), nil)
	if res.Status != copilot.StatusError || res.Data != nil || strings.Contains(res.Message, "plan lookup failed") {
		t.Fatalf("res = %+v", res)
	}
}

func TestEloNeverWidensADepartmentMembersServiceMessages(t *testing.T) {
	messages := &fakeServiceMessages{}
	tool := NewAttendanceServiceMessagesTool(serviceMessageDeps(messages))

	res := tool.Execute(context.Background(), tooltest.MemberOf(tooltest.KnownDepartment), map[string]interface{}{"department_id": "all"})
	if res.Status != copilot.StatusOK || messages.queries[0].Filter.DepartmentID != tooltest.KnownDepartment {
		t.Fatalf("status = %v queries = %+v, want the member's only department", res.Status, messages.queries)
	}
	res = tool.Execute(context.Background(), tooltest.MemberOf(tooltest.KnownDepartment), map[string]interface{}{"department_id": otherDepartment})
	if res.Status != copilot.StatusDenied || len(messages.queries) != 1 {
		t.Fatalf("status = %v queries = %d, want denied before any query", res.Status, len(messages.queries))
	}
	res = tool.Execute(context.Background(), tooltest.MemberOf(), nil)
	if res.Status != copilot.StatusDenied || len(messages.queries) != 1 {
		t.Fatalf("status = %v, want a member in no department denied", res.Status)
	}
}

func TestEloReadsServiceMessagesWithAttendancePermission(t *testing.T) {
	tool := NewAttendanceServiceMessagesTool(AttendanceDeps{})
	if meta := tool.Meta(); meta != attendanceReadMeta() {
		t.Fatalf("meta = %+v", meta)
	}
	if tool.Definition().Name != "attendance_service_messages" {
		t.Fatalf("name = %q", tool.Definition().Name)
	}
	if _, ok := tool.Definition().Parameters["origin"]; !ok {
		t.Fatalf("the tool does not take the attendance filters")
	}
}
