package crm_telemetry_usecase

import (
	"testing"
	"time"

	"vozko/domain/crm_telemetry"
)

func TestAnUnresolvedDepartmentSurvivesTheTelemetryHop(t *testing.T) {
	hist := &memHistory{}
	c := NewConsumerWithDeps(ConsumerDeps{
		Events: &memEvents{}, History: hist, AIRepo: newMemAI(), QueueRepo: &memQueue{}, Presence: &memPresence{}, Dedupe: &memDedupe{},
	}).(*consumer)

	err := c.applyAssignmentHistory(crm_telemetry.AssignmentHistoryPayload{
		WorkspaceID: "ws", EntryID: "e1", EntryType: "whatsapp", ActorKind: "human", AssignedActorID: "u1",
		Trigger: "manual", DepartmentUnresolved: true, StartedAt: time.Now().UTC(),
	})

	if err != nil || len(hist.appended) != 1 {
		t.Fatalf("appended %d, %v", len(hist.appended), err)
	}
	if !hist.appended[0].DepartmentUnresolved || hist.appended[0].DepartmentID != "" {
		t.Fatalf("history = %+v, want the department recorded as unresolved", hist.appended[0])
	}
}
