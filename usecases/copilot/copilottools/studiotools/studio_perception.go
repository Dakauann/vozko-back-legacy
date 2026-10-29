package studiotools

import (
	"encoding/json"
	"errors"
	"log"

	"vozko/domain/copilot"
	"vozko/domain/studio/perception"
)

type StudioPerception interface {
	Checks(facts *perception.Facts) ([]string, error)
	Look(facts *perception.Facts) (map[string]any, error)
	ArtboardAlerts(facts *perception.Facts) ([][]string, error)
	VideoAlerts(facts *perception.Facts) (map[string]any, error)
}

const (
	perceptionField          = "perception"
	studioChecksUnavailable  = "a conferência de design não pôde rodar desta vez e nada foi conferido; não conclua que está tudo certo: confira o resultado com studio_look"
	studioReportsUnavailable = "os alertas e relatórios de design não puderam ser medidos desta vez; isso não quer dizer que não há problemas: confira olhando a imagem com atenção"
)

var errNoPerception = errors.New("studio: the editor reply carried no perception facts or no analysis is wired")

type replyFields map[string]json.RawMessage

func fieldsOf(raw []byte) (replyFields, bool) {
	fields := replyFields{}
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

func (f replyFields) set(key string, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	f[key] = encoded
}

func (f replyFields) encoded(fallback []byte) []byte {
	out, err := json.Marshal(f)
	if err != nil {
		return fallback
	}
	return out
}

func (f replyFields) takeFacts(analysis StudioPerception) (*perception.Facts, error) {
	raw, carried := f[perceptionField]
	delete(f, perceptionField)
	if !carried || analysis == nil {
		return nil, errNoPerception
	}
	return perception.DecodeFacts(raw)
}

func logPerception(command string, err error) {
	log.Printf("[copilot] studio %s could not measure the design: %v", command, err)
}

func (d StudioDeps) withChecks(raw []byte) []byte {
	fields, ok := fieldsOf(raw)
	if !ok {
		return raw
	}
	delete(fields, "checks")
	facts, err := fields.takeFacts(d.Perception)
	var lines []string
	if err == nil {
		lines, err = d.Perception.Checks(facts)
	}
	if err != nil {
		logPerception("edit", err)
		fields.set("checks_unavailable", studioChecksUnavailable)
		return fields.encoded(raw)
	}
	fields.set("checks", lines)
	return fields.encoded(raw)
}

func (d StudioDeps) withReadAlerts(raw []byte, kind copilot.StudioKind) []byte {
	fields, ok := fieldsOf(raw)
	if !ok {
		return raw
	}
	facts, err := fields.takeFacts(d.Perception)
	if err == nil && kind == copilot.StudioImage {
		err = d.mergeArtboardAlerts(fields, facts)
	} else if err == nil {
		var data map[string]any
		if data, err = d.Perception.VideoAlerts(facts); err == nil {
			for key, value := range data {
				fields.set(key, value)
			}
		}
	}
	if err != nil {
		logPerception("read", err)
		fields.set("alerts_unavailable", studioReportsUnavailable)
	}
	return fields.encoded(raw)
}

func (d StudioDeps) mergeArtboardAlerts(fields replyFields, facts *perception.Facts) error {
	alerts, err := d.Perception.ArtboardAlerts(facts)
	if err != nil {
		return err
	}
	var boards []map[string]json.RawMessage
	if err := json.Unmarshal(fields["artboards"], &boards); err != nil || len(boards) != len(alerts) {
		return perception.ErrIncomplete
	}
	for i, lines := range alerts {
		if len(lines) == 0 {
			continue
		}
		encoded, err := json.Marshal(lines)
		if err != nil {
			return err
		}
		boards[i]["alerts"] = encoded
	}
	fields.set("artboards", boards)
	return nil
}

func lookWithReports(target *copilot.ScreenTarget) bool {
	return target != nil && (target.Kind == copilot.LookArtboard || target.Kind == copilot.LookMoments)
}

func mergeReport(fields replyFields, key string, value any) {
	incoming, err := json.Marshal(value)
	if err != nil {
		return
	}
	var existing, added map[string]json.RawMessage
	if json.Unmarshal(fields[key], &existing) == nil && existing != nil && json.Unmarshal(incoming, &added) == nil && added != nil {
		for k, v := range added {
			existing[k] = v
		}
		fields.set(key, existing)
		return
	}
	fields[key] = incoming
}

func (d StudioDeps) withLookReports(reply copilot.ScreenReply) []byte {
	fields, ok := fieldsOf(reply.Data)
	if !ok {
		return reply.Data
	}
	facts, err := fields.takeFacts(d.Perception)
	if !lookWithReports(reply.Target) {
		if err == nil && facts.Unmarked {
			fields.set("marks_unavailable", perception.MarksUnavailable)
		}
		return fields.encoded(reply.Data)
	}
	var data map[string]any
	if err == nil {
		data, err = d.Perception.Look(facts)
	}
	if err != nil {
		logPerception("look", err)
		fields.set("alerts_unavailable", studioReportsUnavailable)
		return fields.encoded(reply.Data)
	}
	for key, value := range data {
		mergeReport(fields, key, value)
	}
	return fields.encoded(reply.Data)
}
