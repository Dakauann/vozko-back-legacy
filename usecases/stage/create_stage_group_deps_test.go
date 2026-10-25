package stage_usecase

import (
	"errors"
	"testing"

	"vozko/domain/shared/deps"
)

func TestCreatingAStageGroupRequiresItsRepositoriesAndDepartmentResolver(t *testing.T) {
	uc, err := NewCreateStageGroupUseCase(nil, nil, nil)
	var missing *deps.MissingError
	if uc != nil || !errors.As(err, &missing) || len(missing.Missing) != 3 {
		t.Fatalf("uc = %v err = %v, want all three dependencies refused", uc, err)
	}
}
