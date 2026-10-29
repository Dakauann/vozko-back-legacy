package sitetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/working_hours"
	site_usecase "vozko/usecases/site"
)

const bookingValue = `{"duration_minutes":30,"horizon_days":14,"title":"Avaliação com {nome}","consent_text":"Autorizo o contato.","hours":{"timezone":"America/Sao_Paulo","days":{"mon":[{"start":"09:00","end":"12:00"}]}}}`

func TestReferencesListTheBookingBlocksOfThePage(t *testing.T) {
	r := newRig(t)
	r.storage.settings.Settings.Bookings = map[string]site.Booking{"agenda": {
		DurationMinutes: 30, HorizonDays: 14, Title: "Avaliação", ConsentText: "Ok",
		Hours: working_hours.Spec{Timezone: "America/Sao_Paulo", Days: map[string][]working_hours.Window{"mon": {{Start: "09:00", End: "12:00"}}}},
	}}
	r.deps.References = &fakeReferences{catalog: site_usecase.ReferenceCatalog{GoogleCalendar: true}}
	raw, _ := json.Marshal(NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"references":true}`)).Data)
	if !strings.Contains(string(raw), `"booking_blocks":[{"id":"agenda","name":"30 min, America/Sao_Paulo"}]`) || !strings.Contains(string(raw), `"google_calendar_connected":true`) {
		t.Fatalf("booking blocks missing in %s", raw)
	}
}

func TestSiteSettingsWritesOneBookingBlockByItsID(t *testing.T) {
	r := newRig(t)
	tool := NewSiteSettingsTool(r.deps)
	res := tool.Execute(context.Background(), r.cc, map[string]interface{}{"section": "bookings", "form_id": "agenda", "value": bookingValue})
	if res.Status != copilot.StatusOK {
		t.Fatalf("result %+v", res)
	}
	saved := r.storage.settings.Settings.Bookings["agenda"]
	if saved.DurationMinutes != 30 || saved.Hours.Timezone != "America/Sao_Paulo" {
		t.Fatalf("saved %+v", saved)
	}
	read := decoded(t, tool.Execute(context.Background(), r.cc, map[string]interface{}{"section": "bookings", "form_id": "agenda"}).Data)
	if value, _ := json.Marshal(read["value"]); !strings.Contains(string(value), `"duration_minutes":30`) {
		t.Fatalf("read %v", read)
	}
	bad := tool.Execute(context.Background(), r.cc, map[string]interface{}{"section": "bookings", "form_id": "agenda", "value": strings.Replace(bookingValue, `"duration_minutes":30`, `"duration_minutes":3`, 1)})
	if bad.Status == copilot.StatusOK || !strings.Contains(bad.Message, site.CodeBookingDurationInvalid) {
		t.Fatalf("an invalid booking was saved: %+v", bad)
	}
	gone := tool.Execute(context.Background(), r.cc, map[string]interface{}{"section": "bookings", "form_id": "agenda", "value": "null"})
	if gone.Status != copilot.StatusOK || len(r.storage.settings.Settings.Bookings) != 0 {
		t.Fatalf("removal %+v %+v", gone, r.storage.settings.Settings.Bookings)
	}
}
