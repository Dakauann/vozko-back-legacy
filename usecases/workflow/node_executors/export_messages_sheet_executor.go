package node_executors

import (
	"context"
	"log"
	"strings"

	"vozko/domain/conversation"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	gsuc "vozko/usecases/googlesheet"
)

type SheetMessageExports interface {
	Start(ctx context.Context, target gsuc.ExportTarget) (*googlesheet.Sheet, googlesheet.ExportPosition, error)
	Record(ctx context.Context, in gsuc.ExportInput) (*gsuc.ExportResult, error)
}

type ConversationMessagesSince interface {
	MessagesSince(entryID string, entryType shared.EntryType, since conversation.MessageKeyset, limit int) ([]*conversation.Message, error)
}

type exportMessagesSheetExecutor struct {
	exports  SheetMessageExports
	messages ConversationMessagesSince
}

func NewExportMessagesSheetExecutor(exports SheetMessageExports, messages ConversationMessagesSince) workflow.NodeExecutor {
	return &exportMessagesSheetExecutor{exports: exports, messages: messages}
}

func (e *exportMessagesSheetExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{
		Type:              workflow.NodeTypeActionExportMessagesSheet,
		TestBehavior:      workflow.NodeTestExternalCall,
		Category:          workflow.NodeCategoryAction,
		Scopes:            []workflow.NodeScope{workflow.NodeScopeShared},
		NeedsConversation: true,
		Label:             "Exportar mensagens para planilha",
		Description:       "Grava na planilha uma linha por mensagem da conversa, desde a última exportação desta conversa para a mesma planilha.",
		Icon:              "Table",
		Guidance: workflow.NodeGuidance{
			When: "Para registrar a linha do tempo de um cliente numa planilha do Google Planilhas, por exemplo quando a conversa entra na etapa de pagamento recebido.",
			Behavior: "Lê as mensagens novas da conversa desde a última exportação para esta planilha e grava todas de uma vez, uma linha por mensagem, na ordem em que aconteceram. " +
				"As colunas com fonte de mensagem (data, quem enviou, sentido, texto, link da mídia) mudam a cada linha; as do lead e da conversa se repetem em todas. " +
				"Repetir o passo nunca duplica linhas. A saída 'erro' recebe recusas da planilha, e nesse caso nada é marcado como exportado.",
			Examples: []string{
				"config: {\"sheet_id\":\"<id>\",\"values\":{\"valor\":\"{{var.valor}}\"}}",
			},
		},
		Outputs: []workflow.HandleDefinition{
			{ID: addSheetRowWritten, Label: "Sucesso"},
			{ID: addSheetRowRefused, Label: "Erro", Optional: true},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "exported", Description: "Quantas mensagens novas entraram na fila de gravação", Label: "Mensagens exportadas", Type: workflow.VariableTypeNumber},
			{Key: "duplicates", Description: "Mensagens que já estavam na planilha e não foram repetidas", Label: "Já exportadas", Type: workflow.VariableTypeNumber},
			{Key: "has_more", Description: "true quando há mais mensagens do que cabem em uma exportação; a próxima exportação continua delas", Label: "Há mais mensagens", Type: workflow.VariableTypeBoolean},
			{Key: "sheet_id", Description: "ID da planilha", Label: "ID da planilha", Type: workflow.VariableTypeText},
			{Key: "sheet_title", Description: "Título da planilha", Label: "Título da planilha", Type: workflow.VariableTypeText},
			{Key: "sheet_url", Description: "Link da planilha no Google", Label: "Link da planilha", Type: workflow.VariableTypeText},
			{Key: "error", Description: "Descrição da recusa", Label: "Recusa", Type: workflow.VariableTypeText},
			{Key: "error_code", Description: "Código da recusa", Label: "Código da recusa", Type: workflow.VariableTypeText},
		},
		DefaultConfig: map[string]interface{}{AddSheetRowSheetKey: "", AddSheetRowValuesKey: map[string]interface{}{}},
		ConfigSchema: []workflow.ConfigField{
			{Key: AddSheetRowSheetKey, Label: "Planilha", Type: "select", OptionsSource: "google_sheets", Required: true},
			{Key: AddSheetRowValuesKey, Label: "Colunas", Type: "sheet_columns",
				Description: "O valor das colunas preenchidas pelo fluxo, igual em todas as linhas desta exportação. Aceita variáveis do fluxo {{...}}."},
		},
	}
}

func (e *exportMessagesSheetExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	if ctx == nil || ctx.Run == nil || ctx.Node == nil {
		return nil, workflow.ErrNodeConfigMissing
	}
	if e.exports == nil || e.messages == nil {
		return sheetRowRefused(ctx, googlesheet.CodeInternal, "integração Google Planilhas indisponível no servidor"), nil
	}
	if strings.TrimSpace(ctx.Run.EntryID) == "" || ctx.Run.OnLead() {
		return sheetRowRefused(ctx, googlesheet.CodeNeedsConversation, googlesheet.ErrNeedsConversation.Error()), nil
	}
	values, missing := sheetValues(ctx)
	if len(missing) > 0 {
		return sheetRowRefused(ctx, googlesheet.CodeUnresolvedVariable, "variáveis sem valor: "+strings.Join(missing, ", ")), nil
	}
	sheetID, _ := ctx.Node.Config[AddSheetRowSheetKey].(string)
	target := gsuc.ExportTarget{WorkspaceID: ctx.Run.WorkspaceID, SheetID: strings.TrimSpace(sheetID), Origin: workflowSheetOrigin(ctx.Run)}

	callCtx, release := ctx.CallContext()
	defer release()
	sheet, position, err := e.exports.Start(callCtx, target)
	if err != nil {
		return e.failed(ctx, err)
	}
	read, err := e.messages.MessagesSince(ctx.Run.EntryID, shared.EntryType(ctx.Run.EntryType), conversation.MessageKeyset{At: position.At, ID: position.MessageID}, googlesheet.MaxExportMessages)
	if err != nil {
		log.Printf("[workflow][node:%s][run:%s] export_messages_sheet could not read the conversation: %v", ctx.Node.ID, ctx.Run.ID, err)
		return nil, err
	}
	output := map[string]interface{}{
		"exported": 0, "duplicates": 0, "has_more": len(read) >= googlesheet.MaxExportMessages,
		"sheet_id": sheet.ID, "sheet_title": sheet.Title, "sheet_url": sheet.URL(),
	}
	if len(read) == 0 {
		return routeOutput(&workflow.NodeResult{Output: output}, outgoingEdges(ctx), addSheetRowWritten), nil
	}
	exported := make([]googlesheet.ExportedMessage, 0, len(read))
	for _, message := range read {
		if visible, ok := googlesheet.ExportedMessageOf(message); ok {
			exported = append(exported, visible)
		}
	}
	last := read[len(read)-1]
	result, err := e.exports.Record(callCtx, gsuc.ExportInput{
		Target: target, Values: values, Messages: exported,
		Through: googlesheet.ExportPosition{At: last.CreatedAt, MessageID: last.ID},
	})
	if err != nil {
		return e.failed(ctx, err)
	}
	output["exported"], output["duplicates"] = result.Exported, result.Duplicates
	return routeOutput(&workflow.NodeResult{Output: output}, outgoingEdges(ctx), addSheetRowWritten), nil
}

func (e *exportMessagesSheetExecutor) failed(ctx *workflow.NodeContext, err error) (*workflow.NodeResult, error) {
	code := sheetRowCode(err)
	if code == googlesheet.CodeInternal && canRetryNode(ctx) {
		log.Printf("[workflow][node:%s][run:%s] export_messages_sheet will retry the same messages: %v", ctx.Node.ID, ctx.Run.ID, err)
		return nil, err
	}
	return sheetRowRefused(ctx, code, err.Error()), nil
}
