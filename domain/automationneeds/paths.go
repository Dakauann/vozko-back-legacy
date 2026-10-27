package automationneeds

import "vozko/domain/conversation"

var slotPaths = map[SlotKind]string{
	SlotWhatsAppNumber: "/dashboard/whatsapp-business-phones/",
	SlotUnofficial:     "/dashboard/unofficial-whatsapp/",
	SlotInstagram:      "/dashboard/instagram-accounts/",
	SlotFacebook:       "/dashboard/facebook-pages/",
	SlotTelegram:       "/dashboard/telegram-accounts/",
	SlotTikTok:         "/dashboard/tiktok-accounts/",
	SlotMercadoLivre:   "/dashboard/mercadolivre-accounts/",
	SlotWebchat:        "/dashboard/webchat/",
}

var slotHomes = map[SlotKind]string{
	SlotEmail: "/dashboard/email",
	SlotRCS:   "/dashboard/rcs-sms/rcs-senders",
	SlotSMS:   "/dashboard/rcs-sms/sms-senders",
}

var slotLabels = map[SlotKind]string{
	SlotWhatsAppNumber: "número do WhatsApp",
	SlotUnofficial:     "número do WhatsApp não oficial",
	SlotInstagram:      "conta do Instagram",
	SlotFacebook:       "página do Facebook",
	SlotTelegram:       "bot do Telegram",
	SlotTikTok:         "conta do TikTok",
	SlotMercadoLivre:   "conta do Mercado Livre",
	SlotWebchat:        "chat do site",
	SlotEmail:          "caixa de email",
	SlotRCS:            "remetente RCS",
	SlotSMS:            "rota de SMS",
}

func SlotPath(kind SlotKind, id string) string {
	if prefix, ok := slotPaths[kind]; ok {
		return prefix + id
	}
	return slotHomes[kind]
}

func SlotLabel(kind SlotKind) string {
	return slotLabels[kind]
}

func AutomationPath(a conversation.Automation) string {
	if a.Kind == conversation.AutomationWorkflow {
		return "/dashboard/workflows/" + a.ID
	}
	return "/dashboard/agents/" + a.ID
}

func AutomationNoun(a conversation.Automation) string {
	if a.Kind == conversation.AutomationWorkflow {
		return "o fluxo"
	}
	return "o agente"
}
