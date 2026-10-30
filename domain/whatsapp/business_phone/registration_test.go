package businessphone

import "testing"

func TestANumberIsRegisteredUnlessItIsAlreadyConnectedInTheAccountBeingConnected(t *testing.T) {
	linkedTo := func(wabaID string) *WhatsAppBusinessPhoneNumber {
		return &WhatsAppBusinessPhoneNumber{MetaPhoneNumberID: "phone-1", WABAId: wabaID}
	}
	cases := []struct {
		name        string
		linked      *WhatsAppBusinessPhoneNumber
		metaStatus  Status
		coexistence bool
		want        bool
	}{
		{"a number still waiting for registration", nil, StatusPending, false, true},
		{"a new number Meta already connected", nil, StatusConnected, false, false},
		{"a number already connected in this account", linkedTo("waba-new"), StatusConnected, false, false},
		{"a number moving in from another account finishes its move on registration", linkedTo("waba-old"), StatusConnected, false, true},
		{"a number shared with the WhatsApp Business app is never registered", linkedTo("waba-old"), StatusConnected, true, false},
		{"a pending number shared with the app is never registered either", nil, StatusPending, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RegistrationNeeded(c.linked, "waba-new", c.metaStatus, c.coexistence); got != c.want {
				t.Fatalf("RegistrationNeeded = %v, want %v", got, c.want)
			}
		})
	}
}
