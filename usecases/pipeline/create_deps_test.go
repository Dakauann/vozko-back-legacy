package pipeline_usecase

import (
	"errors"
	"testing"

	"vozko/domain/pipeline"
	"vozko/domain/shared/deps"
)

type recordingSeeder struct{ seeded []string }

func (s *recordingSeeder) SeedConversationPipeline(_, pipelineID, _ string, _ []pipeline.StageSeed) error {
	s.seeded = append(s.seeded, pipelineID)
	return nil
}

func createPipeline(repo pipeline.Repository, departments DepartmentDirectory) *CreatePipelineUseCase {
	uc, err := NewCreatePipelineUseCase(repo, departments, &recordingSeeder{})
	if err != nil {
		panic(err)
	}
	return uc
}

func TestCreatingAPipelineRequiresTheStageSeeder(t *testing.T) {
	uc, err := NewCreatePipelineUseCase(newDefaultRepo(), departmentsOf{}, nil)
	var missing *deps.MissingError
	if uc != nil || !errors.As(err, &missing) || missing.Missing[0] != "stage seeder" {
		t.Fatalf("uc = %v err = %v, want the stage seeder refused", uc, err)
	}
}
