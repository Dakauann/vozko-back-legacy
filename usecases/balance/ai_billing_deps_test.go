package balance_usecase

import (
	"errors"
	"testing"

	"vozko/domain/aiusage"
	"vozko/domain/balance"
	"vozko/domain/messaging"
	"vozko/domain/metrics"
	"vozko/domain/shared/deps"
	workspace_pricing "vozko/domain/workspace/workspace_pricing"
)

func aiBillingConsumer(sub messaging.MessageQueueSub, balances balance.Repository, pricer workspace_pricing.Pricer, recorder metrics.BillingMetricsRecorder, usage aiusage.Recorder) *ConsumeAIBillingUseCase {
	if recorder == nil {
		recorder = &stubBillingMetrics{}
	}
	if usage == nil {
		usage = &recordingUsage{}
	}
	consumer, err := NewConsumeAIBillingUseCase(AIBillingDeps{Subscriber: sub, Balances: balances, Pricer: pricer, Metrics: recorder, Usage: usage})
	if err != nil {
		panic(err)
	}
	return consumer
}

func TestNewConsumeAIBillingUseCaseRefusesAMissingUsageRecorder(t *testing.T) {
	consumer, err := NewConsumeAIBillingUseCase(AIBillingDeps{Subscriber: newMockSub(), Balances: newMockBalanceRepo(0), Pricer: newAITestPricer(), Metrics: &stubBillingMetrics{}})
	var missing *deps.MissingError
	if consumer != nil || !errors.As(err, &missing) || missing.Missing[0] != "usage recorder" {
		t.Fatalf("consumer = %v err = %v, want the usage recorder refused", consumer, err)
	}
}
