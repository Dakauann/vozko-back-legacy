package stage_usecase

import (
	"errors"
	"testing"

	ce "vozko/domain/conversation_event"
	"vozko/domain/pipeline"
	"vozko/domain/stage"
)

type conversationFunnels struct{}

func (conversationFunnels) GetByID(workspaceID, id string) (*pipeline.Pipeline, error) {
	return &pipeline.Pipeline{ID: id, WorkspaceID: workspaceID, Name: id, ObjectType: pipeline.ObjectConversation}, nil
}

type funnelBook struct {
	funnels map[string]*pipeline.Pipeline
	err     error
	reads   []string
}

func (f *funnelBook) GetByID(workspaceID, id string) (*pipeline.Pipeline, error) {
	f.reads = append(f.reads, id)
	if f.err != nil {
		return nil, f.err
	}
	p, ok := f.funnels[id]
	if !ok || p.WorkspaceID != workspaceID {
		return nil, pipeline.ErrNotFound
	}
	return p, nil
}

func altivaFunnels() *funnelBook {
	return &funnelBook{funnels: map[string]*pipeline.Pipeline{
		"pl-atendimento": {ID: "pl-atendimento", WorkspaceID: "ws", Name: "Atendimento", ObjectType: pipeline.ObjectConversation},
		"pl-altiva":      {ID: "pl-altiva", WorkspaceID: "ws", Name: "Altiva", ObjectType: pipeline.ObjectConversation},
		"pl-deals":       {ID: "pl-deals", WorkspaceID: "ws", Name: "Vendas", ObjectType: pipeline.ObjectOpportunity},
	}}
}

func altivaRepo() *coherenceRepo {
	repo := newCoherenceRepo()
	repo.stages["s-novo"] = &stage.Stage{ID: "s-novo", WorkspaceID: "ws", PipelineID: "pl-atendimento", Name: "novo"}
	repo.stages["s-triagem"] = &stage.Stage{ID: "s-triagem", WorkspaceID: "ws", PipelineID: "pl-atendimento", Name: "triagem"}
	repo.stages["s-pago"] = &stage.Stage{ID: "s-pago", WorkspaceID: "ws", PipelineID: "pl-altiva", Name: "pagamento efetuado"}
	repo.stages["s-deal"] = &stage.Stage{ID: "s-deal", WorkspaceID: "ws", PipelineID: "pl-deals", Name: "proposta"}
	repo.stages["s-alheia"] = &stage.Stage{ID: "s-alheia", WorkspaceID: "outra-ws", PipelineID: "pl-x", Name: "alheia"}
	repo.entryStage = &stage.EntryStage{StageID: "s-novo", StageName: "novo"}
	return repo
}

func crossFunnelInput(stageID string) stage.AssignEntryStageInput {
	return stage.AssignEntryStageInput{StageID: stageID, EntryID: "e-1", EntryType: "whatsapp", AllowCrossPipeline: true}
}

func TestACrossFunnelMoveNamesTheFunnelItLeftAndTheOneItEntered(t *testing.T) {
	repo := altivaRepo()
	move, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", crossFunnelInput("s-pago"))
	if err != nil {
		t.Fatalf("an explicit cross-funnel move must go through: %v", err)
	}
	if len(repo.assigned) != 1 || repo.assigned[0].StageID != "s-pago" {
		t.Fatalf("assigned = %+v, want the conversation on pagamento efetuado", repo.assigned)
	}
	want := stage.FunnelStage{StageID: "s-pago", StageName: "pagamento efetuado", PipelineID: "pl-altiva", PipelineName: "Altiva"}
	if move.To != want {
		t.Fatalf("to = %+v, want %+v", move.To, want)
	}
	from := stage.FunnelStage{StageID: "s-novo", StageName: "novo", PipelineID: "pl-atendimento", PipelineName: "Atendimento"}
	if move.From != from {
		t.Fatalf("from = %+v, want %+v", move.From, from)
	}
	if !move.FunnelChanged() || move.Entry == nil {
		t.Fatalf("move = %+v, want a funnel change with the placement", move)
	}
}

func TestAMoveInsideTheFunnelReadsOnlyTheDestinationFunnel(t *testing.T) {
	repo := altivaRepo()
	funnels := altivaFunnels()
	move, err := NewAssignEntryStageUseCase(repo, funnels, nil).Move("ws", stage.AssignEntryStageInput{StageID: "s-triagem", EntryID: "e-1", EntryType: "whatsapp"})
	if err != nil {
		t.Fatal(err)
	}
	if move.FunnelChanged() || move.From.PipelineID != "pl-atendimento" || move.From.StageName != "novo" {
		t.Fatalf("move = %+v, want an in-funnel move from novo", move)
	}
	if len(funnels.reads) != 1 {
		t.Fatalf("funnel reads = %v, an in-funnel move needs only the destination", funnels.reads)
	}
}

func TestACrossFunnelMoveWithoutThePermissionIsStillRefused(t *testing.T) {
	repo := altivaRepo()
	input := crossFunnelInput("s-pago")
	input.AllowCrossPipeline = false
	_, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", input)
	if !errors.Is(err, stage.ErrStagePipelineMismatch) {
		t.Fatalf("err = %v, want ErrStagePipelineMismatch", err)
	}
	if len(repo.assigned) != 0 {
		t.Fatal("a refused move must not write")
	}
}

func TestAConversationNeverLandsOnAnOpportunityStage(t *testing.T) {
	for name, current := range map[string]*stage.EntryStage{
		"from another funnel": {StageID: "s-novo"},
		"first placement":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			repo := altivaRepo()
			repo.entryStage = current
			_, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", crossFunnelInput("s-deal"))
			if !errors.Is(err, stage.ErrStageNotConversationFunnel) {
				t.Fatalf("err = %v, want ErrStageNotConversationFunnel", err)
			}
			if len(repo.assigned) != 0 {
				t.Fatal("an opportunity stage must never hold a conversation")
			}
		})
	}
}

func TestAnotherWorkspacesStageStaysRefusedAcrossFunnels(t *testing.T) {
	repo := altivaRepo()
	_, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", crossFunnelInput("s-alheia"))
	if !errors.Is(err, stage.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if len(repo.assigned) != 0 {
		t.Fatal("a cross-tenant move must never write")
	}
}

func TestAStageWhoseFunnelIsNotInTheWorkspaceIsRefused(t *testing.T) {
	repo := altivaRepo()
	repo.stages["s-orfa"] = &stage.Stage{ID: "s-orfa", WorkspaceID: "ws", PipelineID: "pl-sumiu", Name: "órfã"}
	_, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", crossFunnelInput("s-orfa"))
	if !errors.Is(err, stage.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if len(repo.assigned) != 0 {
		t.Fatal("a stage of an unknown funnel must not be written")
	}
}

func TestAMoveFailsClosedWhenTheFunnelCannotBeRead(t *testing.T) {
	repo := altivaRepo()
	funnels := altivaFunnels()
	funnels.err = errors.New("db down")
	if _, err := NewAssignEntryStageUseCase(repo, funnels, nil).Move("ws", crossFunnelInput("s-pago")); err == nil {
		t.Fatal("an unreadable funnel must refuse the move")
	}
	if len(repo.assigned) != 0 {
		t.Fatal("an unverified move must not write")
	}
}

func TestAMoveFailsClosedWithoutAFunnelReader(t *testing.T) {
	repo := altivaRepo()
	if _, err := NewAssignEntryStageUseCase(repo, nil, nil).Move("ws", crossFunnelInput("s-pago")); err == nil {
		t.Fatal("a move that cannot check the funnel must be refused")
	}
	if len(repo.assigned) != 0 {
		t.Fatal("an unverified move must not write")
	}
}

type unreadablePlacementRepo struct{ *coherenceRepo }

func (unreadablePlacementRepo) GetEntryStage(string, string, string) (*stage.EntryStage, error) {
	return nil, errors.New("db down")
}

func TestAMoveFailsClosedWhenTheCurrentStageCannotBeRead(t *testing.T) {
	repo := altivaRepo()
	if _, err := NewAssignEntryStageUseCase(unreadablePlacementRepo{repo}, altivaFunnels(), nil).Move("ws", crossFunnelInput("s-pago")); err == nil {
		t.Fatal("an unreadable placement must refuse the move")
	}
	if len(repo.assigned) != 0 {
		t.Fatal("an unverified move must not write")
	}
}

func TestADeletedCurrentStageLeavesNoFunnelToCross(t *testing.T) {
	repo := altivaRepo()
	repo.entryStage = &stage.EntryStage{StageID: "s-apagada", StageName: "apagada"}
	move, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Move("ws", stage.AssignEntryStageInput{StageID: "s-pago", EntryID: "e-1", EntryType: "whatsapp"})
	if err != nil {
		t.Fatalf("a conversation on a deleted stage must still be placeable: %v", err)
	}
	if move.FunnelChanged() || move.From.StageName != "apagada" {
		t.Fatalf("move = %+v", move)
	}
}

func TestTheDestinationNamesTheStageAndItsFunnel(t *testing.T) {
	uc := NewAssignEntryStageUseCase(altivaRepo(), altivaFunnels(), nil)
	got, err := uc.Destination("ws", "s-pago")
	if err != nil {
		t.Fatal(err)
	}
	if got != (stage.FunnelStage{StageID: "s-pago", StageName: "pagamento efetuado", PipelineID: "pl-altiva", PipelineName: "Altiva"}) {
		t.Fatalf("destination = %+v", got)
	}
	if _, err := uc.Destination("ws", "s-deal"); !errors.Is(err, stage.ErrStageNotConversationFunnel) {
		t.Fatalf("an opportunity stage is no destination for a conversation: %v", err)
	}
	if _, err := uc.Destination("ws", "s-alheia"); !errors.Is(err, stage.ErrUnauthorized) {
		t.Fatalf("another workspace's stage is no destination: %v", err)
	}
	if _, err := uc.Destination("ws", "s-sumiu"); !errors.Is(err, stage.ErrTagNotFound) {
		t.Fatalf("a missing stage is no destination: %v", err)
	}
}

func TestACrossFunnelMoveNamesBothFunnelsOnTheTimeline(t *testing.T) {
	repo := altivaRepo()
	log := &recordingLogger{}
	if _, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), log).Move("ws", crossFunnelInput("s-pago")); err != nil {
		t.Fatal(err)
	}
	d := log.ofType(ce.EventStageChanged).DetailsMap()
	want := map[string]string{
		"from_stage_name":    "novo",
		"stage_name":         "pagamento efetuado",
		"from_pipeline_id":   "pl-atendimento",
		"from_pipeline_name": "Atendimento",
		"pipeline_id":        "pl-altiva",
		"pipeline_name":      "Altiva",
	}
	for key, value := range want {
		if d[key] != value {
			t.Fatalf("details[%s] = %q, want %q (details %v)", key, d[key], value, d)
		}
	}
}

func TestAnInFunnelMoveKeepsTheTimelineFreeOfFunnels(t *testing.T) {
	repo := altivaRepo()
	log := &recordingLogger{}
	if _, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), log).Move("ws", stage.AssignEntryStageInput{StageID: "s-triagem", EntryID: "e-1", EntryType: "whatsapp"}); err != nil {
		t.Fatal(err)
	}
	d := log.ofType(ce.EventStageChanged).DetailsMap()
	for _, key := range []string{"from_pipeline_id", "from_pipeline_name", "pipeline_id", "pipeline_name"} {
		if _, ok := d[key]; ok {
			t.Fatalf("details = %v, an in-funnel move names no funnel", d)
		}
	}
}

func TestExecuteIsTheMovesPlacement(t *testing.T) {
	repo := altivaRepo()
	got, err := NewAssignEntryStageUseCase(repo, altivaFunnels(), nil).Execute("ws", crossFunnelInput("s-pago"))
	if err != nil || got == nil || got.StageID != "s-pago" {
		t.Fatalf("Execute = %+v, %v", got, err)
	}
}

func TestACrossFunnelMoveByAWorkflowIsAnnouncedLikeAnyMoveWithItsCause(t *testing.T) {
	repo := altivaRepo()
	heard := &heardEntries{}
	input := crossFunnelInput("s-pago")
	input.Cause = stage.MoveCause{WorkflowID: "wf-altiva", RunID: "run-1", Chain: 1}
	if _, err := NewAssignEntryStageUseCase(AnnounceEntries(repo, heard), altivaFunnels(), nil).Move("ws", input); err != nil {
		t.Fatal(err)
	}
	if len(heard.entered) != 1 {
		t.Fatalf("heard %+v, want one stage-added announcement", heard.entered)
	}
	got := heard.entered[0]
	if got.StageID != "s-pago" || got.FromStageID != "s-novo" || got.Cause != input.Cause {
		t.Fatalf("heard %+v, want the move into pagamento efetuado carrying the workflow that made it", got)
	}
}
