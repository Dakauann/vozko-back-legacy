package workflow

import (
	"fmt"
	"sort"
	"strings"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/domain/variabledecl"
)

type ChannelFact string

const (
	ChannelFactContactPhone ChannelFact = "contact_phone"
	ChannelFactContactEmail ChannelFact = "contact_email"
	ChannelFactPhoneHandle  ChannelFact = "phone_handle"
	ChannelFactTemplates    ChannelFact = "whatsapp_templates"
	ChannelFactForms        ChannelFact = "whatsapp_forms"
	ChannelFactChoices      ChannelFact = "interactive_choices"
)

type ChannelLevel string

const (
	ChannelGuaranteed ChannelLevel = "guaranteed"
	ChannelMaybe      ChannelLevel = "maybe"
	ChannelNever      ChannelLevel = "never"
)

var phoneChannelFacts = map[ChannelFact]ChannelLevel{
	ChannelFactContactPhone: ChannelGuaranteed, ChannelFactContactEmail: ChannelMaybe, ChannelFactPhoneHandle: ChannelGuaranteed,
	ChannelFactForms: ChannelNever, ChannelFactChoices: ChannelGuaranteed,
}

var socialChannelFacts = map[ChannelFact]ChannelLevel{
	ChannelFactContactPhone: ChannelMaybe, ChannelFactContactEmail: ChannelMaybe, ChannelFactPhoneHandle: ChannelNever,
	ChannelFactForms: ChannelNever, ChannelFactChoices: ChannelGuaranteed,
}

var channelGuarantees = map[shared.EntryType]map[ChannelFact]ChannelLevel{
	shared.EntryTypeWhatsApp: {
		ChannelFactContactPhone: ChannelGuaranteed, ChannelFactContactEmail: ChannelMaybe, ChannelFactPhoneHandle: ChannelGuaranteed,
		ChannelFactForms: ChannelGuaranteed, ChannelFactChoices: ChannelGuaranteed,
	},
	shared.EntryTypeUnofficialWhatsApp: phoneChannelFacts,
	shared.EntryTypeRCS:                phoneChannelFacts,
	shared.EntryTypeSMS: {
		ChannelFactContactPhone: ChannelGuaranteed, ChannelFactContactEmail: ChannelMaybe, ChannelFactPhoneHandle: ChannelGuaranteed,
		ChannelFactForms: ChannelNever, ChannelFactChoices: ChannelNever,
	},
	shared.EntryTypeInstagram: socialChannelFacts,
	shared.EntryTypeFacebook:  socialChannelFacts,
	shared.EntryTypeTelegram:  socialChannelFacts,
	shared.EntryTypeWebchat:   socialChannelFacts,
	shared.EntryTypeTikTok:    socialChannelFacts,
	shared.EntryTypeMercadoLivre: {
		ChannelFactContactPhone: ChannelMaybe, ChannelFactContactEmail: ChannelMaybe, ChannelFactPhoneHandle: ChannelNever,
		ChannelFactForms: ChannelNever, ChannelFactChoices: ChannelNever,
	},
	shared.EntryTypeEmail: {
		ChannelFactContactPhone: ChannelMaybe, ChannelFactContactEmail: ChannelGuaranteed, ChannelFactPhoneHandle: ChannelNever,
		ChannelFactForms: ChannelNever, ChannelFactChoices: ChannelNever,
	},
}

var derivedChannelFacts = map[ChannelFact]func(shared.EntryType) bool{
	ChannelFactTemplates: shared.EntryType.SupportsTemplates,
}

func ChannelProvides(channel shared.EntryType, fact ChannelFact) ChannelLevel {
	if derived, ok := derivedChannelFacts[fact]; ok {
		if _, known := channelGuarantees[channel]; known && derived(channel) {
			return ChannelGuaranteed
		}
		return ChannelNever
	}
	level, known := channelGuarantees[channel][fact]
	if !known {
		return ChannelNever
	}
	return level
}

func IsChannel(entryType string) bool {
	_, known := channelGuarantees[shared.EntryType(entryType)]
	return known
}

func ChannelFacts() []ChannelFact {
	return []ChannelFact{ChannelFactContactPhone, ChannelFactContactEmail, ChannelFactPhoneHandle, ChannelFactTemplates, ChannelFactForms, ChannelFactChoices}
}

func ChannelsWith(fact ChannelFact, level ChannelLevel) []shared.EntryType {
	var out []shared.EntryType
	for _, channel := range ChannelBranchOrder {
		if ChannelProvides(channel, fact) == level {
			out = append(out, channel)
		}
	}
	return out
}

var channelFactNouns = map[ChannelFact]string{
	ChannelFactContactPhone: "o telefone do contato",
	ChannelFactContactEmail: "o e-mail do contato",
	ChannelFactPhoneHandle:  "um número de telefone em Contato › número",
	ChannelFactTemplates:    "templates do WhatsApp oficial",
	ChannelFactForms:        "formulários do WhatsApp oficial",
	ChannelFactChoices:      "botões e listas de opções",
}

func (f ChannelFact) Noun() string {
	return channelFactNouns[f]
}

func ChannelLabels(channels []shared.EntryType) string {
	labels := make([]string, 0, len(channels))
	for _, channel := range channels {
		labels = append(labels, ChannelBranchLabel(channel))
	}
	return strings.Join(labels, ", ")
}

type ChannelNeed struct {
	Fact     ChannelFact
	Required bool
	Fallback string
	Field    string
	Path     string
	Subject  string
}

var nodeChannelNeeds = map[NodeType][]ChannelNeed{
	NodeTypeActionSendTemplate:    {{Fact: ChannelFactTemplates, Required: true, Field: "template_id"}},
	NodeTypeActionSendForm:        {{Fact: ChannelFactForms, Required: true, Fallback: FormHandleSendFailed, Field: FormNodeFormIDKey}},
	NodeTypeActionSendInteractive: {{Fact: ChannelFactChoices, Required: true, Fallback: InteractiveHandleSendFailed}},
	NodeTypeActionSendCarrier:     {{Fact: ChannelFactContactPhone, Required: true, Fallback: CarrierHandleFailed}},
}

type channelVariable struct {
	fact    ChannelFact
	subject string
}

var channelVariables = map[string]channelVariable{
	DataKeyContactNumber:                        {fact: ChannelFactPhoneHandle, subject: label("Contato", "número")},
	ConversationRef(variabledecl.KeyLeadNumber): {fact: ChannelFactContactPhone, subject: systemLabelOf(variabledecl.KeyLeadNumber)},
}

func systemLabelOf(name string) string {
	text, _ := variabledecl.SystemLabel(name)
	return text
}

type SheetChannelColumn struct {
	Column   string
	Header   string
	Fact     ChannelFact
	Required bool
}

type SheetChannelColumns map[string][]SheetChannelColumn

var sheetSourceFacts = map[googlesheet.Source]ChannelFact{
	googlesheet.SourceLeadPhone: ChannelFactContactPhone,
	googlesheet.SourceLeadEmail: ChannelFactContactEmail,
}

func SheetChannelColumnsOf(columns []googlesheet.Column) []SheetChannelColumn {
	var out []SheetChannelColumn
	for _, col := range columns {
		fact, depends := sheetSourceFacts[col.Source]
		if !depends {
			continue
		}
		out = append(out, SheetChannelColumn{Column: col.Key, Header: col.Header, Fact: fact, Required: col.Required})
	}
	return out
}

func NodeChannelNeeds(node *Node, columns SheetChannelColumns) []ChannelNeed {
	if node == nil {
		return nil
	}
	needs := append([]ChannelNeed(nil), nodeChannelNeeds[node.Type.Canonical()]...)
	if node.Type == NodeTypeActionAddSheetRow || node.Type == NodeTypeActionExportMessagesSheet {
		for _, col := range columns[SheetStepTab(node)] {
			needs = append(needs, ChannelNeed{
				Fact: col.Fact, Required: col.Required, Fallback: SheetStepRefused,
				Field: SheetStepValuesKey, Path: col.Column, Subject: col.Header,
			})
		}
	}
	for _, read := range fieldReadsOf(node) {
		if v, depends := channelVariables[read.ref]; depends {
			needs = append(needs, ChannelNeed{Fact: v.fact, Field: read.field, Path: read.ref, Subject: v.subject})
		}
	}
	return needs
}

func SortedChannels(set map[shared.EntryType]bool) []shared.EntryType {
	out := make([]shared.EntryType, 0, len(set))
	for channel := range set {
		out = append(out, channel)
	}
	rank := map[shared.EntryType]int{}
	for i, channel := range ChannelBranchOrder {
		rank[channel] = i + 1
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := rank[out[i]], rank[out[j]]
		if ri != rj {
			return ri < rj
		}
		return out[i] < out[j]
	})
	return out
}

type VariableChannelNote struct {
	Key           string   `json:"key" enums:"contact_phone,contact_email,phone_handle" example:"phone_handle"`
	Text          string   `json:"text" example:"É um telefone só em WhatsApp oficial, WhatsApp não oficial, RCS, SMS. Em Instagram, Telegram traz o identificador do contato no canal."`
	Channels      []string `json:"channels" example:"instagram,telegram"`
	ChannelLabels string   `json:"channelLabels" example:"Instagram, Telegram"`
	Guaranteed    string   `json:"guaranteed" example:"WhatsApp oficial, SMS"`
}

var channelNoteTexts = map[ChannelFact]string{
	ChannelFactPhoneHandle:  "É um telefone só em %s. Em %s traz o identificador do contato no canal, não um telefone.",
	ChannelFactContactPhone: "Sempre preenchido em %s. Em %s fica vazio quando o contato ainda não informou o telefone.",
	ChannelFactContactEmail: "Sempre preenchido em %s. Em %s fica vazio quando o contato ainda não informou o e-mail.",
}

func ChannelNoteOf(ref string) *VariableChannelNote {
	v, depends := channelVariables[canonicalRef(ref)]
	if !depends {
		return nil
	}
	lacking := make([]string, 0, len(ChannelBranchOrder))
	var lackingChannels []shared.EntryType
	for _, channel := range ChannelBranchOrder {
		if ChannelProvides(channel, v.fact) != ChannelGuaranteed {
			lacking = append(lacking, string(channel))
			lackingChannels = append(lackingChannels, channel)
		}
	}
	guaranteed := ChannelLabels(ChannelsWith(v.fact, ChannelGuaranteed))
	text := fmt.Sprintf(channelNoteTexts[v.fact], guaranteed, ChannelLabels(lackingChannels))
	return &VariableChannelNote{Key: string(v.fact), Text: text, Channels: lacking, ChannelLabels: ChannelLabels(lackingChannels), Guaranteed: guaranteed}
}
