package attendancetools

import (
	"context"
	"errors"

	"vozko/domain/attendance"
	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
)

const serviceMessagesTool = "attendance_service_messages"

type attendanceServiceMessagesTool struct{ deps AttendanceDeps }

func NewAttendanceServiceMessagesTool(deps AttendanceDeps) copilot.Tool {
	return &attendanceServiceMessagesTool{deps: deps}
}

func (t *attendanceServiceMessagesTool) Meta() copilot.Meta { return attendanceReadMeta() }

func (t *attendanceServiceMessagesTool) Definition() tools.Definition {
	return tools.Definition{
		Name: serviceMessagesTool,
		Description: "Mensagens de serviço do WhatsApp oficial no período: as respostas livres (equipe ou IA) que a Meta cobrou, " +
			"fora da janela gratuita de anúncio. Só existe para workspaces cujo plano cobra mensagens de serviço; para os demais a " +
			"ferramenta responde que o plano não cobra, e então não estime nem invente o número. Traz o total, o período de maior " +
			"volume, o valor estimado em reais ao preço atual do plano (spend_estimate_brl, nulo sem cotação) e um dataset por dia, " +
			"semana ou mês para render_chart. Com outro canal no filtro, applicable vem false e o total vem nulo.",
		Parameters: attendanceParams(),
	}
}

func (t *attendanceServiceMessagesTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	q, err := t.deps.resolve(ctx, cc, args)
	if err != nil {
		return analyticsFailure(serviceMessagesTool, err)
	}
	section, err := t.deps.ServiceMessages.ServiceMessages(ctx, cc.WorkspaceID, attendance.ServiceMessageQuery{Filter: q.filter})
	switch {
	case errors.Is(err, attendance.ErrServiceMessagesNotOnPlan):
		return copilot.Result{Status: copilot.StatusError, Message: "o plano deste workspace não cobra mensagens de serviço, então esse número não está disponível; diga isso ao usuário sem estimar"}
	case errors.Is(err, attendance.ErrServiceMessagePriceUnavailable):
		return copilot.Result{Status: copilot.StatusError, Message: "não consegui conferir agora se o plano cobra mensagens de serviço; tente de novo em instantes"}
	case err != nil:
		return analyticsFailure(serviceMessagesTool, err)
	}
	dataset, err := serviceMessagesDataset(*section)
	if err != nil {
		return analyticsFailure(serviceMessagesTool, err)
	}
	return copilot.Result{Status: copilot.StatusOK, Data: map[string]interface{}{
		"query":            q.describe(),
		"service_messages": attendance.DigestServiceMessages(*section),
		"dataset":          copilottools.Keep(cc, dataset).Preview(),
	}}
}

func serviceMessagesDataset(s attendance.ServiceMessagesSection) (*copilot.Dataset, error) {
	period := string(s.Granularity)
	if period == "" {
		period = string(attendance.GranularityDay)
	}
	d := copilot.NewDataset("service_messages", []copilot.Column{
		{Key: period, Label: period, Kind: copilot.ColumnText},
		{Key: "service_messages", Label: "service_messages", Kind: copilot.ColumnNumber},
	})
	for _, bucket := range s.Buckets {
		if err := d.AddRow(bucket.Bucket, bucket.Messages); err != nil {
			return nil, err
		}
	}
	return d, nil
}
