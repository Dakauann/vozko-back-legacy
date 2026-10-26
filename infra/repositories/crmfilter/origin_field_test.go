package crmfilter

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lib/pq"

	"vozko/domain/conversation"
	"vozko/domain/crmfilter"
	"vozko/infra/database/sqlexpr"
)

func TestTheConversationOriginFilterUsesTheSharedPrimaryKeyProbe(t *testing.T) {
	desc := NewConversationDescriptor()
	in := "(" + sqlexpr.ConversationOriginIn("ae.entry_id", "ae.entry_type") + ")"
	cases := []struct {
		name     string
		p        crmfilter.Predicate
		wantArgs []interface{}
	}{
		{"in", pred(crmfilter.FieldOrigin, crmfilter.OpIn, "receptive", "campaign"),
			[]interface{}{pq.Array([]string{"receptive", "campaign"})}},
		{"eq", pred(crmfilter.FieldOrigin, crmfilter.OpEquals, "receptive"),
			[]interface{}{pq.Array([]string{"receptive"})}},
		{"not in keeps the other recorded origins", pred(crmfilter.FieldOrigin, crmfilter.OpNotIn, "outreach"),
			[]interface{}{pq.Array([]string{"receptive", "campaign", "unknown"})}},
		{"neq", pred(crmfilter.FieldOrigin, crmfilter.OpNotEquals, "campaign"),
			[]interface{}{pq.Array([]string{"receptive", "outreach", "unknown"})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Compile(crmfilter.Filter{Groups: []crmfilter.Group{group(crmfilter.And, tc.p)}}, desc, 1)
			if err != nil {
				t.Fatal(err)
			}
			if sql != in || !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("got %q %v\nwant %q %v", sql, args, in, tc.wantArgs)
			}
		})
	}
}

func TestAnUnknownOriginIsRefusedAsAnInvalidFilterValue(t *testing.T) {
	_, _, err := Compile(crmfilter.Filter{Groups: []crmfilter.Group{
		group(crmfilter.And, pred(crmfilter.FieldOrigin, crmfilter.OpIn, "receptive", "inbound")),
	}}, NewConversationDescriptor(), 1)
	if !errors.Is(err, crmfilter.ErrInvalidValue) || !errors.Is(err, conversation.ErrOriginInvalid) {
		t.Fatalf("err = %v, want an invalid origin value", err)
	}
}

func TestOnlyConversationsHaveAnOrigin(t *testing.T) {
	for _, desc := range []ObjectDescriptor{NewLeadDescriptor(), NewOpportunityDescriptor()} {
		if _, err := desc.Field(crmfilter.FieldOrigin); !errors.Is(err, crmfilter.ErrNotApplicable) {
			t.Fatalf("%s: origin err = %v, want not applicable", desc.Object(), err)
		}
	}
}
