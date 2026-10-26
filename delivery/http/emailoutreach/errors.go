package emailoutreach

import (
	"net/http"

	"vozko/delivery/http/emailrefusal"
	"vozko/delivery/http/response"
	"vozko/usecases/emailoutreach"
)

var refusalSendFailed = emailrefusal.SendFailed

var outreachRefusals = append(append(append([]response.Refusal{}, emailrefusal.Transactional...), emailrefusal.Hygiene...),
	response.Refusal{Status: http.StatusForbidden, Code: emailoutreach.CodeForbidden, Message: "Este remetente pertence a um departamento fora do seu acesso."},
	response.Refusal{Status: http.StatusServiceUnavailable, Code: emailoutreach.CodeStarterNotAssigned, Message: "O e-mail foi enviado, mas a conversa não pôde ser atribuída a você. Tente de novo com a mesma chave: nada é enviado nem cobrado outra vez."},
)

var refusalByCode = emailrefusal.Index(outreachRefusals)

func RefusalCodes() []string {
	return response.RefusalCodes(outreachRefusals, []response.Refusal{refusalSendFailed})
}

func writeStartRefusal(w http.ResponseWriter, workspaceID string, err error) {
	emailrefusal.Write(w, "[email-outreach]", workspaceID, err, refusalByCode)
}
