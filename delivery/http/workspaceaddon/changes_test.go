package workspaceaddon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
	workspaceaddondomain "vozko/domain/workspace/workspace_addon"
	"vozko/pkg/codegolden"
)

type scheduledQuantity struct {
	err       error
	workspace string
	sub       string
	quantity  int
}

func (s *scheduledQuantity) Execute(workspaceID, subscriptionID string, quantity int) (*workspaceaddondomain.AddonSubscription, error) {
	s.workspace, s.sub, s.quantity = workspaceID, subscriptionID, quantity
	if s.err != nil {
		return nil, s.err
	}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	return &workspaceaddondomain.AddonSubscription{ID: subscriptionID, Quantity: 4, ScheduledQuantity: &quantity, ScheduledFrom: &from}, nil
}

func quantityRouter(h *WorkspaceAddonHandler) (*mux.Router, *[]string) {
	r := mux.NewRouter()
	var seen []string
	RegisterProtectedRoutes(r, h, func(res workspace_domain.Resource, act workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			seen = append(seen, string(res)+":"+string(act))
			next(w, req)
		}
	})
	return r, &seen
}

func TestReducingAnAddonIsScheduledAndGuardedLikeACancellation(t *testing.T) {
	uc := &scheduledQuantity{}
	r, seen := quantityRouter((&WorkspaceAddonHandler{}).WithQuantityChanges(uc))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/workspaces/ws-1/addons/sub-1/quantity", strings.NewReader(`{"quantity":2}`)))
	if w.Code != http.StatusOK || uc.workspace != "ws-1" || uc.sub != "sub-1" || uc.quantity != 2 {
		t.Fatalf("got %d %s %+v", w.Code, w.Body.String(), uc)
	}
	if len(*seen) != 1 || (*seen)[0] != "plans:delete" {
		t.Fatalf("grants = %v", *seen)
	}
	if !strings.Contains(w.Body.String(), `"scheduledQuantity":2`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAddonRefusalsCarryStableCodes(t *testing.T) {
	cases := map[error]string{
		workspaceaddondomain.ErrUnitsInUse:            "addon_units_in_use",
		workspaceaddondomain.ErrAddonUnitCapReached:   "addon_unit_cap_reached",
		workspaceaddondomain.ErrReductionUnsupported:  "addon_reduction_unsupported",
		workspaceaddondomain.ErrMonthlyOnly:           "addon_monthly_only",
		workspaceaddondomain.ErrPurchaseChargeInvalid: "addon_unavailable",
		workspaceaddondomain.ErrMonthlyPriceRequired:  "addon_unavailable",
		errors.New("boom"):                            "addon_changes_unavailable",
	}
	for err, code := range cases {
		r, _ := quantityRouter((&WorkspaceAddonHandler{}).WithQuantityChanges(&scheduledQuantity{err: err}))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/workspaces/ws-1/addons/sub-1/quantity", strings.NewReader(`{"quantity":1}`)))
		if !strings.Contains(w.Body.String(), `"`+code+`"`) {
			t.Fatalf("%v: got %d %s", err, w.Code, w.Body.String())
		}
	}
	r, _ := quantityRouter(&WorkspaceAddonHandler{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/workspaces/ws-1/addons/sub-1/quantity", strings.NewReader(`{"quantity":1}`)))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unwired reduction refuses, got %d", w.Code)
	}
}

func TestAddonErrorCodesMatchTheGolden(t *testing.T) {
	var codes []string
	for _, entry := range addonRefusals {
		codes = append(codes, entry.Refusal.Code)
	}
	for _, refusal := range fallbackAddonRefusals {
		codes = append(codes, refusal.Code)
	}
	codegolden.Compare(t, filepath.Join("testdata", "workspace_addon_error_codes.golden"), codegolden.Unique(codes),
		"update testdata/workspace_addon_error_codes.golden, and translate sites.errors.<code> in the pt, en, es and de catalogs of vozko-front")
}
