package personalinbox

import (
	"errors"
	"testing"
)

func TestEveryoneStartsWithGroupsInTheirOwnTab(t *testing.T) {
	if DefaultPreferences().HideGroups {
		t.Fatal("groups are shown in their own tab until the person hides them")
	}
}

func TestPreferencesBelongToOnePersonInOneWorkspace(t *testing.T) {
	cases := []struct {
		name      string
		workspace string
		user      string
		want      error
	}{
		{"no workspace", " ", "u-1", ErrWorkspaceRequired},
		{"no user", "ws-1", "", ErrUserRequired},
		{"both", "ws-1", "u-1", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidatePreferencesOwner(c.workspace, c.user); !errors.Is(err, c.want) {
				t.Fatalf("ValidatePreferencesOwner = %v, want %v", err, c.want)
			}
		})
	}
}
