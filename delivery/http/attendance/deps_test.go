package attendance

import (
	"errors"
	"testing"

	"vozko/domain/shared/deps"
)

func TestNewAttendanceHandlerRefusesMissingDependencies(t *testing.T) {
	h, err := NewAttendanceHandler(Deps{Sections: &stubSections{}})
	var missing *deps.MissingError
	if h != nil || !errors.As(err, &missing) {
		t.Fatalf("h = %v err = %v, want the missing dependencies refused", h, err)
	}
	for _, name := range []string{"target scoper", "member activity", "frt stats", "queue events", "presence", "targets"} {
		found := false
		for _, m := range missing.Missing {
			found = found || m == name
		}
		if !found {
			t.Errorf("%s is not reported missing in %v", name, missing.Missing)
		}
	}
}
