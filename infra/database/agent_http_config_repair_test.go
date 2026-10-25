package database

import (
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/agenttool"
)

func TestTheHTTPConfigRepairIsDeclaredAfterTheCopy(t *testing.T) {
	names := strings.Join(repairNames(), ",")
	copyAt, canonicalAt := strings.Index(names, "ag_tool_instances"), strings.Index(names, "ag_canonical_http_configs")
	if copyAt < 0 || canonicalAt < copyAt {
		t.Fatalf("repairs %s", names)
	}
	if strings.Count(legacyHTTPToolsSQL, "?") != 2 {
		t.Fatalf("the scan binds only the cursor and the batch, never a jsonb operator: %s", legacyHTTPToolsSQL)
	}
}

func TestALegacyHTTPConfigIsRewrittenCanonically(t *testing.T) {
	raw, ok := canonicalHTTPConfig(legacyHTTPToolRow{ID: "t1", Config: `{"url":"https://a.test/{id}","method":"GET","headers":{"Authorization":"{{secret.k}}"},"path_params":[{"name":"id","type":"string"}]}`})
	if !ok {
		t.Fatal("not converted")
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &config); err != nil || !agenttool.IsCanonicalHTTPConfig(config) {
		t.Fatalf("config %s", raw)
	}
	if _, ok := canonicalHTTPConfig(legacyHTTPToolRow{ID: "t2", Config: "nope"}); ok {
		t.Fatal("an unreadable config is left alone")
	}
}
