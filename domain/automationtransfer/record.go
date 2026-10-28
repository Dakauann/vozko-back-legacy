package automationtransfer

import (
	"strings"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/stage"
	"vozko/domain/workflow"
)

const labelSeparator = " · "

type Record struct {
	From          conversation.Automation
	FromName      string
	To            conversation.Automation
	ToName        string
	DestinationID string
	Note          string
	Stage         stage.FunnelStage
	Filled        []Filled
}

func Label(name string, at stage.FunnelStage) string {
	return joined(name, at.PipelineName, at.StageName)
}

func joined(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, labelSeparator)
}

func (r Record) Details() map[string]string {
	details := map[string]string{
		"from_actor_id":      r.From.ActorID(),
		"from_kind":          string(r.From.Kind),
		"from_automation_id": r.From.ID,
		"from_username":      r.FromName,
		"to_actor_id":        r.To.ActorID(),
		"to_kind":            string(r.To.Kind),
		"to_automation_id":   r.To.ID,
		"to_username":        r.ToName,
		"destination_id":     r.DestinationID,
		"reason":             r.Note,
	}
	if len(r.Filled) > 0 {
		details["filled_variables"] = filledDetail(r.Filled)
	}
	if r.Stage.StageID != "" {
		details["stage_id"] = r.Stage.StageID
		details["stage_name"] = r.Stage.StageName
		details["pipeline_name"] = r.Stage.PipelineName
	}
	return details
}

func RecordOf(details map[string]string) (Record, bool) {
	to, err := conversation.NewAutomation(details["to_kind"], details["to_automation_id"])
	if err != nil {
		return Record{}, false
	}
	from, _ := conversation.NewAutomation(details["from_kind"], details["from_automation_id"])
	record := Record{
		From:          from,
		FromName:      details["from_username"],
		To:            to,
		ToName:        details["to_username"],
		DestinationID: details["destination_id"],
		Note:          details["reason"],
		Filled:        filledOf(details["filled_variables"]),
	}
	if id := details["stage_id"]; id != "" {
		record.Stage = stage.FunnelStage{StageID: id, StageName: details["stage_name"], PipelineName: details["pipeline_name"]}
	}
	return record, true
}

func (r Record) ContextLine(elapsed time.Duration) string {
	from := strings.TrimSpace(r.FromName)
	if from == "" {
		from = "Outra automação"
	}
	line := from + " transferiu esta conversa para você há " + workflow.ElapsedPT(elapsed) + "."
	if place := joined(r.Stage.PipelineName, r.Stage.StageName); place != "" {
		line += " A conversa foi para " + place + "."
	}
	if note := strings.TrimSpace(r.Note); note != "" {
		line += " Motivo e resumo de quem transferiu: " + note
	}
	return line
}
