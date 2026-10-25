package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/workspace"
)

func TestAMissingPermissionIsRefusedWithAStableCode(t *testing.T) {
	mw := NewWorkspaceMiddleware(grantedAccess{workspace.ResourceWhatsAppCampaigns: workspace.ActionCreate}, nil, nil)
	reached := false
	rec := httptest.NewRecorder()

	mw.CheckAccess(workspace.ResourceWhatsAppCampaigns, workspace.ActionStart, func(http.ResponseWriter, *http.Request) { reached = true })(rec, anyAccessRequest("member"))

	var body struct {
		Code     string            `json:"code"`
		Expected map[string]string `json:"expected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if reached || rec.Code != http.StatusForbidden || body.Code != "start_permission_required" || body.Expected["required_permission"] != "whatsapp_campaigns:start" {
		t.Fatalf("reached %v, %d %s", reached, rec.Code, rec.Body.String())
	}
}

func TestEveryActionHasItsOwnPermissionCode(t *testing.T) {
	if workspace.PermissionRequiredCode(workspace.ActionStart) != "start_permission_required" || workspace.PermissionRequiredCode(workspace.ActionUpdate) != "update_permission_required" {
		t.Fatal("permission codes changed")
	}
}
