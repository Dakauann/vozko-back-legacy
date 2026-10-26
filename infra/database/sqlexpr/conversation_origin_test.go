package sqlexpr

import (
	"strings"
	"testing"

	"vozko/domain/conversation"
)

func TestConversationOriginMapsEverySenderTheDomainNames(t *testing.T) {
	want := "CASE m.sender_kind" +
		" WHEN 'contact' THEN 'receptive'" +
		" WHEN 'campaign' THEN 'campaign'" +
		" WHEN 'human' THEN 'outreach' WHEN 'ai' THEN 'outreach' WHEN 'workflow' THEN 'outreach' WHEN 'external' THEN 'outreach'" +
		" ELSE 'unknown' END"
	if got := ConversationOrigin("m"); got != want {
		t.Fatalf("ConversationOrigin = %q\nwant %q", got, want)
	}
	for _, origin := range conversation.Origins() {
		if origin == conversation.OriginUnknown {
			continue
		}
		if !strings.Contains(ConversationOrigin("m"), "THEN "+Literal(string(origin))) {
			t.Fatalf("origin %q is never produced", origin)
		}
	}
}

func TestTheOpeningMessageSkipsSystemNoticesAndImportSeeds(t *testing.T) {
	want := "m.sender_kind NOT IN ('system') AND " + NotSeedPlaceholder("m")
	if got := OpensConversation("m"); got != want {
		t.Fatalf("OpensConversation = %q, want %q", got, want)
	}
}

func TestConversationOriginIsBindsTheOriginOnce(t *testing.T) {
	got := ConversationOriginIs("wce.id", "'whatsapp'")
	want := "EXISTS (SELECT 1 FROM conversation_origins co_f WHERE co_f.entry_id = wce.id AND co_f.entry_type = 'whatsapp' AND co_f.origin = ?)"
	if got != want {
		t.Fatalf("ConversationOriginIs = %q\nwant %q", got, want)
	}
	if strings.Count(got, "?") != 1 {
		t.Fatalf("the predicate must bind exactly the origin: %s", got)
	}
}

func TestCanonicalUUIDCastsOnlyWellFormedText(t *testing.T) {
	want := "CASE WHEN length(s.entry_id) = 36 AND s.entry_id ~ '" + canonicalUUIDPrefix + "' THEN (s.entry_id)::uuid END"
	if got := CanonicalUUID("s.entry_id"); got != want {
		t.Fatalf("CanonicalUUID = %q, want %q", got, want)
	}
}

func TestConversationOriginInBindsOneArrayOfOrigins(t *testing.T) {
	got := ConversationOriginIn("ae.entry_id", "ae.entry_type")
	want := "EXISTS (SELECT 1 FROM conversation_origins co_f WHERE co_f.entry_id = ae.entry_id AND co_f.entry_type = ae.entry_type AND co_f.origin = ANY(?))"
	if got != want {
		t.Fatalf("ConversationOriginIn = %q\nwant %q", got, want)
	}
}

func TestConversationOriginJoinProbesThePrimaryKey(t *testing.T) {
	got := ConversationOriginJoin("co", "te.entry_id", "?")
	want := "LEFT JOIN conversation_origins co ON co.entry_id = te.entry_id AND co.entry_type = ?"
	if got != want {
		t.Fatalf("ConversationOriginJoin = %q\nwant %q", got, want)
	}
}

func TestEveryOriginPredicateCorrelatesOnTheSamePrimaryKey(t *testing.T) {
	match := "co_f.entry_id = x.id AND co_f.entry_type = 'telegram'"
	for name, sql := range map[string]string{
		"is": ConversationOriginIs("x.id", "'telegram'"),
		"in": ConversationOriginIn("x.id", "'telegram'"),
	} {
		if !strings.Contains(sql, match) {
			t.Fatalf("%s does not probe the primary key: %s", name, sql)
		}
	}
}
