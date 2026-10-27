package googlesheet

type PresetColumn struct {
	Key         string
	Type        ColumnType
	Source      Source
	Options     []string
	Colors      map[string]OptionColor
	Required    bool
	Instruction bool
	LinkRef     string
}

type PresetSheet struct {
	Ref       string
	TabPeriod TabPeriod
	Style     StylePreset
	Columns   []PresetColumn
}

type Preset struct {
	Key    string
	Icon   string
	Sheets []PresetSheet
}

func systemColumn(key string, source Source) PresetColumn {
	t, _ := source.ColumnType()
	return PresetColumn{Key: key, Type: t, Source: source}
}

func inputColumn(key string, t ColumnType) PresetColumn {
	return PresetColumn{Key: key, Type: t, Source: SourceInput}
}

func (c PresetColumn) required() PresetColumn {
	c.Required = true
	return c
}

func (c PresetColumn) explained() PresetColumn {
	c.Instruction = true
	return c
}

func choice(key string, options []string, colors map[string]OptionColor) PresetColumn {
	return PresetColumn{Key: key, Type: TypeChoice, Source: SourceInput, Options: options, Colors: colors}
}

func timelineColumns() []PresetColumn {
	return []PresetColumn{
		systemColumn("data", SourceMessageSentAt),
		systemColumn("cliente", SourceLeadName),
		systemColumn("telefone", SourceLeadPhone),
		systemColumn("quem", SourceMessageSender),
		systemColumn("sentido", SourceMessageDirection),
		systemColumn("mensagem", SourceMessageText),
		systemColumn("anexo", SourceMessageMediaURL),
		systemColumn("conversa", SourceConversationLink),
	}
}

var presets = []Preset{
	{
		Key: "cobranca", Icon: "Receipt",
		Sheets: []PresetSheet{
			{Ref: "negociacoes", TabPeriod: PeriodNone, Style: StyleGraphite, Columns: timelineColumns()},
			{Ref: "pagamentos", TabPeriod: PeriodMonth, Style: StyleGreen, Columns: []PresetColumn{
				inputColumn("data_pagamento", TypeDate).required().explained(),
				systemColumn("cliente", SourceLeadName),
				inputColumn("matricula", TypeText).explained(),
				inputColumn("parcelas", TypeNumber),
				choice("forma_pagamento", []string{"pix", "boleto", "cartao", "dinheiro"}, nil),
				inputColumn("valor", TypeMoney).required(),
				choice("baixa", []string{"sim", "nao"}, map[string]OptionColor{"sim": ColorGreen, "nao": ColorRed}),
				inputColumn("observacoes", TypeLongText),
				{Key: "negociacao", Type: TypeLink, Source: SourceExportLink, LinkRef: "negociacoes"},
				systemColumn("registrado_em", SourceWrittenAt),
			}},
		},
	},
	{
		Key: "timeline", Icon: "ChatsCircle",
		Sheets: []PresetSheet{{Ref: "mensagens", TabPeriod: PeriodNone, Style: StyleGraphite, Columns: timelineColumns()}},
	},
	{
		Key: "qualified_leads", Icon: "Target",
		Sheets: []PresetSheet{{Ref: "leads", TabPeriod: PeriodNone, Style: StyleBlue, Columns: []PresetColumn{
			systemColumn("registrado_em", SourceWrittenAt),
			systemColumn("nome", SourceLeadName),
			systemColumn("telefone", SourceLeadPhone),
			systemColumn("email", SourceLeadEmail),
			inputColumn("interesse", TypeText).required().explained(),
			inputColumn("orcamento", TypeMoney),
			choice("temperatura", []string{"quente", "morno", "frio"}, map[string]OptionColor{"quente": ColorRed, "morno": ColorAmber, "frio": ColorBlue}),
			inputColumn("proximo_passo", TypeLongText).explained(),
			systemColumn("origem", SourceOrigin),
			systemColumn("conversa", SourceConversationLink),
		}}},
	},
	{
		Key: "scheduling", Icon: "CalendarCheck",
		Sheets: []PresetSheet{{Ref: "agendamentos", TabPeriod: PeriodMonth, Style: StylePurple, Columns: []PresetColumn{
			systemColumn("registrado_em", SourceWrittenAt),
			systemColumn("cliente", SourceLeadName),
			systemColumn("telefone", SourceLeadPhone),
			inputColumn("data", TypeDate).required(),
			inputColumn("horario", TypeText).required().explained(),
			inputColumn("servico", TypeText),
			inputColumn("profissional", TypeText),
			choice("status", []string{"agendado", "confirmado", "cancelado"}, map[string]OptionColor{"agendado": ColorBlue, "confirmado": ColorGreen, "cancelado": ColorRed}),
			inputColumn("observacoes", TypeLongText),
			systemColumn("conversa", SourceConversationLink),
		}}},
	},
	{
		Key: "support_tickets", Icon: "Headset",
		Sheets: []PresetSheet{{Ref: "chamados", TabPeriod: PeriodMonth, Style: StyleGraphite, Columns: []PresetColumn{
			systemColumn("aberto_em", SourceWrittenAt),
			systemColumn("cliente", SourceLeadName),
			systemColumn("telefone", SourceLeadPhone),
			inputColumn("assunto", TypeText).required(),
			inputColumn("descricao", TypeLongText).explained(),
			choice("prioridade", []string{"alta", "media", "baixa"}, map[string]OptionColor{"alta": ColorRed, "media": ColorAmber, "baixa": ColorBlue}),
			choice("status", []string{"aberto", "em_andamento", "resolvido"}, map[string]OptionColor{"aberto": ColorAmber, "em_andamento": ColorBlue, "resolvido": ColorGreen}),
			systemColumn("conversa", SourceConversationLink),
		}}},
	},
	{
		Key: "deals", Icon: "Handshake",
		Sheets: []PresetSheet{{Ref: "negocios", TabPeriod: PeriodNone, Style: StyleGreen, Columns: []PresetColumn{
			systemColumn("registrado_em", SourceWrittenAt),
			systemColumn("cliente", SourceLeadName),
			systemColumn("telefone", SourceLeadPhone),
			inputColumn("produto", TypeText).required(),
			inputColumn("valor", TypeMoney).required(),
			choice("etapa", []string{"proposta", "negociacao", "ganho", "perdido"}, map[string]OptionColor{"proposta": ColorBlue, "negociacao": ColorAmber, "ganho": ColorGreen, "perdido": ColorRed}),
			inputColumn("previsao_fechamento", TypeDate),
			inputColumn("observacoes", TypeLongText),
			systemColumn("origem", SourceOrigin),
			systemColumn("conversa", SourceConversationLink),
		}}},
	},
	{
		Key: "form_answers", Icon: "ClipboardText",
		Sheets: []PresetSheet{{Ref: "respostas", TabPeriod: PeriodNone, Style: StyleBlue, Columns: []PresetColumn{
			systemColumn("respondido_em", SourceWrittenAt),
			systemColumn("nome", SourceLeadName),
			systemColumn("telefone", SourceLeadPhone),
			systemColumn("email", SourceLeadEmail),
			inputColumn("resposta_1", TypeText).explained(),
			inputColumn("resposta_2", TypeText),
			inputColumn("resposta_3", TypeLongText),
			inputColumn("aceitou_contato", TypeBoolean),
			systemColumn("conversa", SourceConversationLink),
		}}},
	},
}

func Presets() []Preset { return presets }

func PresetByKey(key string) (Preset, bool) {
	for _, preset := range presets {
		if preset.Key == key {
			return preset, true
		}
	}
	return Preset{}, false
}

func PresetCopyKeys() []string {
	var keys []string
	for _, preset := range presets {
		keys = append(keys, preset.Key+".name", preset.Key+".description")
		for _, sheet := range preset.Sheets {
			base := preset.Key + ".sheets." + sheet.Ref
			keys = append(keys, base+".title", base+".description")
			if sheet.TabPeriod == PeriodMonth {
				keys = append(keys, base+".tabPattern")
			}
			for _, col := range sheet.Columns {
				colBase := base + ".columns." + col.Key
				keys = append(keys, colBase+".header")
				if col.Instruction {
					keys = append(keys, colBase+".instruction")
				}
				if col.Source == SourceExportLink {
					keys = append(keys, colBase+".linkLabel")
				}
				for _, option := range col.Options {
					keys = append(keys, colBase+".options."+option)
				}
			}
		}
	}
	return keys
}
