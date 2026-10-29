package copilot_usecase

import (
	"slices"

	"vozko/domain/copilot"
)

type toolArea struct {
	Key     string
	Summary string
	Tools   []string
}

var toolAreaCatalog = []toolArea{
	{Key: "agentes", Summary: "agentes de IA: ler, criar, editar e excluir, com modelos e ferramentas válidos", Tools: []string{
		"count_agents", "create_agent", "delete_agent", "get_agent", "list_agent_tools", "list_agents", "list_models", "update_agent",
	}},
	{Key: "anuncios", Summary: "anúncios da Meta: contas, fundos, páginas, públicos da criação, formulários, rascunhos, criar e publicar, resultados", Tools: []string{
		"ad_account_readiness", "ad_placement_look", "ads_breakdown", "ads_results", "connect_ad_account", "create_ad", "create_lead_form",
		"estimate_ad_audience", "get_ad_creative", "get_ad_draft", "list_ad_accounts", "list_ad_apps", "list_ad_catalogs", "list_ad_drafts",
		"list_ad_pages", "list_lead_forms", "list_page_posts", "publish_ad_draft", "save_ad_draft", "search_ad_interests", "search_ad_locations",
		"set_ad_spend_cap", "update_ad_draft",
	}},
	{Key: "anuncios-conversoes", Summary: "conversões do CRM para a Meta: configurações, conjunto de dados, pixels e envios recentes", Tools: []string{
		"connect_ad_dataset", "create_ad_pixel", "get_ad_conversion_settings", "list_ad_pixels", "recent_ad_conversions", "save_ad_conversion_settings",
	}},
	{Key: "anuncios-gestao", Summary: "mudar anúncios publicados: ligar, desligar, orçamento, textos, conjunto, criativo, duplicar, arquivar, excluir e em lote", Tools: []string{
		"archive_ad", "bulk_change_ads", "bulk_edit_ads_text", "bulk_turn_off_ads", "bulk_turn_on_ads", "delete_ad", "duplicate_ad", "edit_ad_set",
		"edit_ad_text", "swap_ad_creative", "turn_off_ad", "turn_on_ad", "update_ad_budget",
	}},
	{Key: "anuncios-publicos", Summary: "públicos personalizados, semelhantes e salvos da Meta", Tools: []string{
		"create_customer_list_audience", "create_lookalike_audience", "create_saved_audience", "delete_ad_audience", "list_ad_audiences",
	}},
	{Key: "anuncios-relatorios", Summary: "relatórios, regras automáticas e testes A/B de anúncios", Tools: []string{
		"ad_rule_history", "create_ad_rule", "create_ad_test", "delete_ad_rule", "export_ad_report", "list_ad_reports", "list_ad_rules",
		"list_ad_tests", "run_ad_report", "run_saved_ad_report", "set_ad_rule_status",
	}},
	{Key: "atendimento", Summary: "métricas de atendimento, equipe, funis, fila ao vivo, disparos, análises das conversas e atividade de membros", Tools: []string{
		"attendance_backlog", "attendance_live", "attendance_metrics", "attendance_rework", "attendance_service_messages", "attendance_stages",
		"attendance_team", "attendance_trend", "campaign_dispatch", "conversation_insights", "member_activity", "read_choice_stats",
	}},
	{Key: "campanhas", Summary: "campanhas de WhatsApp oficiais (planilha e modelo) e não oficiais (QR code)", Tools: []string{
		"create_campaign", "create_unofficial_campaign", "list_unofficial_numbers", "preview_campaign_import", "preview_unofficial_campaign_import",
		"start_campaign", "start_unofficial_campaign",
	}},
	{Key: "conhecimento", Summary: "bases de conhecimento: listar, buscar, criar e adicionar documentos", Tools: []string{
		"add_knowledge_document", "create_knowledge_base", "list_knowledge_bases", "search_knowledge",
	}},
	{Key: "contatos", Summary: "memórias de contatos, mapa e ações sobre grupos de leads", Tools: []string{
		"add_lead_memory", "cancel_lead_send", "lead_geo_summary", "prepare_lead_action", "start_lead_send", "update_lead_memory",
	}},
	{Key: "conversas", Summary: "operar conversas: etiquetas, etapas, mensagens, modelos, responsáveis, cadastros e automações", Tools: []string{
		"activate_workflow", "apply_label", "assign_conversation", "cancel_scheduled_message", "create_label", "list_assignable_members",
		"list_labels", "list_pipelines", "list_templates", "list_workflows", "move_conversation_funnel", "move_conversation_stage",
		"pause_workflow", "remove_label", "schedule_message", "send_message", "send_template", "transfer_conversation",
	}},
	{Key: "criacao", Summary: "abrir o Estúdio, um e-mail ou um site novo e gerar imagem, música, locução e vídeo", Tools: []string{
		"generate_image", "generate_music", "generate_voiceover", "new_email_design", "new_site", "new_studio_project", "render_video",
	}},
	{Key: "email", Summary: "e-mail marketing: enviar e ver resultados", Tools: []string{
		"email_campaign_metrics", "email_overview", "send_email",
	}},
	{Key: "equipe", Summary: "membros, convites, funções, permissões, departamentos e dúvidas de acesso", Tools: []string{
		"add_department_member", "cancel_invite", "change_member_role", "create_department", "create_role", "delete_department", "delete_role",
		"diagnose_access", "explain_permission", "get_member_permissions", "invite_member", "list_department_members", "list_permission_catalog",
		"list_roles", "list_workspace_invites", "list_workspace_members", "remove_department_member", "remove_member", "update_department",
		"update_member_permissions", "update_role",
	}},
	{Key: "formularios", Summary: "formulários do WhatsApp: criar, publicar, enviar, respostas, resultados e automação", Tools: []string{
		"create_form_automation", "create_whatsapp_form", "export_whatsapp_form_responses", "list_whatsapp_forms", "publish_whatsapp_form",
		"send_whatsapp_form", "whatsapp_form_responses", "whatsapp_form_results",
	}},
	{Key: "funis", Summary: "funis, etapas e negociações", Tools: []string{
		"create_deal", "create_pipeline", "create_stage", "link_deal", "list_deal_pipelines", "list_deals", "move_deal", "rename_stage",
		"reorder_stages", "set_initial_stage",
	}},
	{Key: "google", Summary: "agenda e planilhas do Google", Tools: []string{
		"add_google_sheet_row", "create_calendar_event", "create_google_sheet", "list_calendar_events", "list_google_sheets",
	}},
	{Key: "grupos", Summary: "grupos de WhatsApp: painel, pessoas, análises e campanhas", Tools: []string{
		"create_unofficial_group_analysis", "create_unofficial_group_campaign", "list_unofficial_groups", "unofficial_group_overview",
		"unofficial_group_people", "unofficial_groups_dashboard",
	}},
	{Key: "telefonia", Summary: "ligar, histórico de ligações, linhas telefônicas e filas", Tools: []string{
		"change_phone_line_password", "create_call_queue", "create_phone_line", "delete_call_queue", "delete_phone_line", "get_call",
		"list_call_queues", "list_calls", "list_phone_lines", "place_call", "update_call_queue", "update_phone_line",
	}},
	{Key: "whatsapp", Summary: "modelos do WhatsApp, quem atende cada número e bots do Telegram", Tools: []string{
		"configure_number_automation", "connect_telegram_bot", "create_template", "edit_template", "list_business_phones", "list_telegram_bots",
		"number_automation",
	}},
}

var surfaceAreas = map[copilot.Surface][]string{
	copilot.SurfaceAttendance: {"atendimento"},
	copilot.SurfaceLeads:      {"contatos"},
}

type toolAreas struct {
	areas  []toolArea
	byTool map[string]string
	tools  map[string]copilot.Tool
}

func newToolAreas(lookup func(name string) (copilot.Tool, bool)) toolAreas {
	index := toolAreas{byTool: map[string]string{}, tools: map[string]copilot.Tool{}}
	for _, area := range toolAreaCatalog {
		var present []string
		for _, name := range area.Tools {
			if tool, ok := lookup(name); ok {
				present = append(present, name)
				index.byTool[name] = area.Key
				index.tools[name] = tool
			}
		}
		if len(present) > 0 {
			slices.Sort(present)
			index.areas = append(index.areas, toolArea{Key: area.Key, Summary: area.Summary, Tools: present})
		}
	}
	return index
}

func (a toolAreas) on(view copilot.View) toolAreas {
	scoped := toolAreas{byTool: map[string]string{}, tools: map[string]copilot.Tool{}}
	for _, area := range a.areas {
		var offered []string
		for _, name := range area.Tools {
			if view.Offers(a.tools[name]) {
				offered = append(offered, name)
				scoped.byTool[name] = area.Key
				scoped.tools[name] = a.tools[name]
			}
		}
		if len(offered) > 0 {
			scoped.areas = append(scoped.areas, toolArea{Key: area.Key, Summary: area.Summary, Tools: offered})
		}
	}
	return scoped
}

func (a toolAreas) of(name string) string {
	return a.byTool[name]
}

func (a toolAreas) known(key string) bool {
	return slices.ContainsFunc(a.areas, func(area toolArea) bool { return area.Key == key })
}

func (a toolAreas) keys() []string {
	keys := make([]string, 0, len(a.areas))
	for _, area := range a.areas {
		keys = append(keys, area.Key)
	}
	return keys
}

func defersOn(view copilot.View) bool {
	return !view.Focused() || view.OnStudio()
}

func defaultAreas(view copilot.View) []string {
	return surfaceAreas[view.Surface]
}
