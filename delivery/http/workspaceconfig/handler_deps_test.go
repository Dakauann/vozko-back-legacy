package workspaceconfig

import "testing"

func TestTheConfigHandlerRefusesAMissingGeocodingCoverage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a config handler without the geocoding coverage must fail boot")
		}
	}()
	NewWorkspaceConfigHandler(nil, nil, nil, nil)
}
