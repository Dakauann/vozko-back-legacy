package conversation

import "testing"

func TestASendTheProviderAcceptedIsRecordedAsSent(t *testing.T) {
	cases := []struct {
		name    string
		outcome *SendOutcome
		want    DeliveryStatus
	}{
		{"no outcome", nil, DeliveryStatusSent},
		{"an outcome without a status", &SendOutcome{ProviderMessageID: "3EB0"}, DeliveryStatusSent},
		{"an outcome the provider already delivered", &SendOutcome{DeliveryStatus: DeliveryStatusDelivered}, DeliveryStatusDelivered},
		{"an outcome the provider refused", &SendOutcome{DeliveryStatus: DeliveryStatusFailed}, DeliveryStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.outcome.RecordedStatus(); got != tc.want {
				t.Fatalf("RecordedStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOnlyAJSONObjectIsMetadataAClaimCanMerge(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		merges   bool
	}{
		{"no metadata", "", false},
		{"an object", `{"interactive":{"kind":"list"}}`, true},
		{"an array", `[1,2]`, false},
		{"broken json", `{"interactive"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Message{Metadata: []byte(tc.metadata)}
			patch, ok := m.MetadataObject()
			if ok != tc.merges {
				t.Fatalf("MetadataObject() ok = %v, want %v", ok, tc.merges)
			}
			if ok && string(patch) != tc.metadata {
				t.Fatalf("patch = %s, want %s", patch, tc.metadata)
			}
		})
	}
}
