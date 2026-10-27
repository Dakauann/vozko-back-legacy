package automationneeds

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrReceptiveUnmet      = errors.New("automation needs: a receptive channel cannot supply the variables this automation needs")
	ErrConversationMissing = errors.New("automation needs: this conversation has no value for variables the automation needs")
	ErrPublishSwitchesOff  = errors.New("automation needs: publishing switches receptive channels off; confirm to go on")
	ErrVoiceVariables      = errors.New("automation needs: a call carries no variables, so a voice workflow cannot need them")
)

type Refusal struct {
	Cause      error
	Automation Link
	Needs      Needs
	Values     map[string]string
	Impact     []SwitchOff
}

func (r *Refusal) Error() string {
	names := strings.Join(r.Needs.Names(), ", ")
	if r.Automation.ID == "" {
		return fmt.Sprintf("%v: %s", r.Cause, names)
	}
	return fmt.Sprintf("%v: %s %s needs %s", r.Cause, r.Automation.Kind, r.Automation.ID, names)
}

func (r *Refusal) Unwrap() error { return r.Cause }

func RefusalOf(err error) (*Refusal, bool) {
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return refusal, true
	}
	return nil, false
}

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrReceptiveUnmet, "automation_variables_unmet"},
	{ErrConversationMissing, "automation_variables_missing"},
	{ErrPublishSwitchesOff, "automation_publish_switches_off"},
	{ErrVoiceVariables, "automation_voice_variables"},
	{ErrUnreadable, "automation_needs_unreadable"},
	{ErrNotFound, "automation_needs_not_found"},
}

func ErrorCode(err error) string {
	for _, known := range errorCodes {
		if errors.Is(err, known.err) {
			return known.code
		}
	}
	return ""
}

func ErrorCodes() []string {
	out := make([]string, 0, len(errorCodes))
	for _, known := range errorCodes {
		out = append(out, known.code)
	}
	return out
}

func (r *Refusal) Sentence() string {
	who := capitalize(AutomationNoun(r.Automation.Automation))
	if r.Automation.Name != "" {
		who += " " + r.Automation.Name
	}
	names := strings.Join(r.Needs.Names(), ", ")
	switch {
	case errors.Is(r.Cause, ErrReceptiveUnmet):
		return who + " precisa das variáveis " + names + ", que conversas receptivas não trazem. Dê um valor padrão a elas ou escolha outra automação."
	case errors.Is(r.Cause, ErrConversationMissing):
		return who + " precisa das variáveis " + names + " e esta conversa não tem valor para elas. Preencha os valores antes de passar a conversa."
	case errors.Is(r.Cause, ErrPublishSwitchesOff):
		return fmt.Sprintf("Publicar desliga a automação em %d canal(is) receptivo(s), porque ela passa a precisar de %s. A publicação precisa ser confirmada no editor.", len(r.Impact), names)
	case errors.Is(r.Cause, ErrVoiceVariables):
		return who + " precisa das variáveis " + names + ", mas ligações não trazem variáveis. Dê um valor padrão a elas."
	}
	return r.Error()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
