package workflow_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/carriermessaging"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	"vozko/domain/workflow"
	"vozko/usecases/workflow/node_executors"
)

type silentLines struct{}

type nothingStoredHere struct{}

func (nothingStoredHere) OwnsURL(string) bool { return false }

func externalMediaURLs() *conversation.MediaURLResolver {
	return &conversation.MediaURLResolver{Storage: nothingStoredHere{}}
}

func (silentLines) Log(*ce.ConversationEvent) {}

type utcZones struct{}

func (utcZones) Location(context.Context, string, string) (*time.Location, error) {
	return time.UTC, nil
}

func TestProductionExecutorsRefuseToStartWithoutTheRefusalLineLogger(t *testing.T) {
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{}); !errors.Is(err, ErrConversationEventsRequired) {
		t.Fatalf("got %v, want ErrConversationEventsRequired", err)
	}
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{ConversationEvents: silentLines{}}); !errors.Is(err, ErrFormSenderRequired) {
		t.Fatalf("got %v, want ErrFormSenderRequired", err)
	}
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}}); !errors.Is(err, ErrMediaURLsRequired) {
		t.Fatalf("got %v, want ErrMediaURLsRequired", err)
	}
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}, MediaURLs: externalMediaURLs()}); !errors.Is(err, ErrAIAttendanceRequired) {
		t.Fatalf("got %v, want ErrAIAttendanceRequired", err)
	}
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}, MediaURLs: externalMediaURLs(), AIAttendance: sandboxRuntime{}}); !errors.Is(err, ErrFollowUpsRequired) {
		t.Fatalf("got %v, want ErrFollowUpsRequired", err)
	}
	if err := RegisterProductionExecutors(NewNodeExecutorRegistry(), ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}, MediaURLs: externalMediaURLs(), AIAttendance: sandboxRuntime{}, FollowUps: simFollowUps{effects: newSimEffects()}}); !errors.Is(err, ErrWorkspaceZonesRequired) {
		t.Fatalf("got %v, want ErrWorkspaceZonesRequired", err)
	}
	registry := NewNodeExecutorRegistry()
	if err := RegisterProductionExecutors(registry, ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}, MediaURLs: externalMediaURLs(), AIAttendance: sandboxRuntime{}, FollowUps: simFollowUps{effects: newSimEffects()}, WorkspaceZones: utcZones{}, VariableRules: acceptingRules{}}); err != nil {
		t.Fatalf("got %v", err)
	}
	for _, nodeType := range []workflow.NodeType{workflow.NodeTypeActionSendText, workflow.NodeTypeActionSendForm, workflow.NodeTypeActionScheduleMessage, workflow.NodeTypeActionCancelScheduledMessages} {
		if _, ok := registry.Get(nodeType); !ok {
			t.Fatalf("%s was not registered", nodeType)
		}
	}
}

func TestSimulationsRecordRefusedSendsNowhereOnPurpose(t *testing.T) {
	if simulationEvents == nil {
		t.Fatal("simulations need an explicit no-op recorder")
	}
	simulationEvents.Log(ce.New("ws", "entry", "whatsapp", ce.EventLeadSendRefused).Build())
}

func TestCarrierNodeExistsOnlyWhileAChannelIsLaunched(t *testing.T) {
	registry := NewNodeExecutorRegistry()
	if err := RegisterProductionExecutors(registry, ExecutorDeps{ConversationEvents: silentLines{}, Forms: node_executors.NewFormSenderRelay(), ChoiceStats: &spyChoiceRecorder{}, MediaURLs: externalMediaURLs(), AIAttendance: sandboxRuntime{}, FollowUps: simFollowUps{effects: newSimEffects()}, WorkspaceZones: utcZones{}, VariableRules: acceptingRules{}}); err != nil {
		t.Fatal(err)
	}
	_, registered := registry.Get(workflow.NodeTypeActionSendCarrier)
	if launched := len(carriermessaging.LaunchedChannels()) > 0; registered != launched {
		t.Fatalf("carrier node registered = %v while a channel launched is %v", registered, launched)
	}
}

type acceptingRules struct{}

func (acceptingRules) Rejected(context.Context, string, map[string]string) ([]string, error) {
	return nil, nil
}
