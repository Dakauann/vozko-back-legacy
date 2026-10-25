package tools_usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/rag"
	"vozko/domain/tools"
	"vozko/usecases/agentctx"
)

const SearchKnowledgeBaseToolName = "search_knowledge_base"

type KnowledgeBaseFinder interface {
	FindByIDs(ctx context.Context, ids []string) ([]*rag.KnowledgeBase, error)
}

type searchKnowledgeBaseTool struct {
	rag   rag.RAGService
	bases KnowledgeBaseFinder
}

const searchKnowledgeBaseScopeKey = "knowledge_base_ids"

func (t *searchKnowledgeBaseTool) ValidateBinding(ctx context.Context, workspaceID string, config map[string]interface{}) error {
	ids := scopedBaseIDs(config)
	if len(ids) == 0 {
		return nil
	}
	found, err := t.bases.FindByIDs(ctx, ids)
	if err != nil {
		return err
	}
	owned := map[string]bool{}
	for _, kb := range found {
		if kb != nil && kb.WorkspaceID == workspaceID {
			owned[kb.ID] = true
		}
	}
	for _, id := range ids {
		if !owned[id] {
			return tools.Invalid(agenttool.CodeKnowledgeBaseInvalid, "uma base escolhida não existe neste workspace")
		}
	}
	return nil
}

func scopedBaseIDs(config map[string]interface{}) []string {
	raw, _ := config[searchKnowledgeBaseScopeKey].([]interface{})
	var ids []string
	for _, item := range raw {
		if id, ok := item.(string); ok && strings.TrimSpace(id) != "" {
			ids = append(ids, strings.TrimSpace(id))
		}
	}
	return ids
}

func NewSearchKnowledgeBaseToolUseCase(ragService rag.RAGService, bases KnowledgeBaseFinder) tools.Handler {
	if ragService == nil {
		return nil
	}
	if bases == nil {
		panic("search_knowledge_base: missing knowledge base finder")
	}
	return &searchKnowledgeBaseTool{rag: ragService, bases: bases}
}

func (t *searchKnowledgeBaseTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               SearchKnowledgeBaseToolName,
		DisplayName:        "Buscar na base de conhecimento",
		DisplayDescription: "Permite ao agente pesquisar ativamente na base de conhecimento durante a conversa, reformulando a busca quando necessário.",
		Description: `Busca na base de conhecimento do agente e retorna os trechos mais relevantes usando busca semântica (por significado) — a consulta não precisa repetir as palavras exatas dos documentos. Retorna trechos ordenados por relevância, numerados e com a fonte de cada um; não retorna documentos inteiros e não cobre assuntos fora da base (conhecimento geral, notícias).

QUANDO USAR:
- O cliente perguntou sobre produtos, preços, políticas, prazos, condições ou qualquer dado que deva estar nos documentos da empresa → busque ANTES de responder, não confie na memória
- O contexto já fornecido não cobre a pergunta ou está incompleto

COMO FORMULAR A CONSULTA:
- Use os termos-chave do assunto em uma frase específica (ex.: 'valor da mensalidade do plano empresarial'), nunca palavras soltas e genéricas
- A mensagem do cliente raramente é a melhor consulta: extraia O QUE ele quer saber e busque por isso
- Para fatos diferentes, faça uma busca separada e focada para cada fato

SE NÃO ENCONTRAR (obrigatório antes de desistir):
1. Reformule com sinônimos e termos que a empresa usaria nos documentos (ex.: 'mensalidade' → 'preço do plano', 'valor')
2. Tente uma versão mais ampla ou mais específica da consulta
3. Só depois de 2-3 buscas sem nada relevante, considere o assunto fora da base

QUANDO NÃO USAR:
- Conversa casual (saudações, agradecimentos) sem pergunta factual
- A informação já está no contexto desta conversa

Esgotadas as buscas sem resultado relevante: responda com base nas suas próprias instruções e no contexto da conversa; se a informação não estiver em lugar nenhum, diga ao cliente que não tem essa informação em vez de inventar.`,
		Parameters: map[string]tools.Parameter{
			"query": {
				Type:               "string",
				Description:        "A consulta de busca. Use os termos-chave do assunto em uma frase específica em linguagem natural (ex.: 'política de reembolso para planos anuais'), nunca palavras soltas e genéricas. Se uma busca anterior não encontrou nada, NÃO repita a mesma consulta: reformule com sinônimos ou termos mais amplos/específicos. Para fatos diferentes, chame a ferramenta várias vezes com uma consulta focada em cada fato.",
				DisplayName:        "Consulta",
				DisplayDescription: "O que buscar na base de conhecimento",
			},
		},
		Required:   []string{"query"},
		Visibility: []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:   tools.CategoryAgentUtility,
		Instances:  tools.Many(),
		ConfigSchema: map[string]tools.ConfigParameter{
			searchKnowledgeBaseScopeKey: {
				Type:               "array",
				DisplayName:        "Bases pesquisadas",
				DisplayDescription: "Vazio: pesquisa todas as bases do agente. Com bases escolhidas, esta busca só olha para elas.",
				OptionsSource:      "knowledge_bases",
			},
		},
	}
}

func (t *searchKnowledgeBaseTool) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return t.ExecuteWithConfig(ctx, nil, params)
}

func (t *searchKnowledgeBaseTool) ExecuteWithConfig(ctx context.Context, config, params map[string]interface{}) (tools.ExecutionResult, error) {
	agentID, _ := config["__agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	var ragCfg *agent.RAGConfig
	if ctxAgent, ok := agentctx.AgentFromContext(ctx); ok {
		if agentID == "" {
			agentID = ctxAgent.ID
		}
		ragCfg = ctxAgent.RAGConfig
	}
	if agentID == "" {
		return errResult("Não foi possível identificar o agente atual. Esta ferramenta só funciona durante um atendimento."), nil
	}

	query, _ := params["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return errResult("O parâmetro 'query' é obrigatório. Informe uma pergunta ou frase específica para buscar."), nil
	}

	cfg := ragCfg.WithDefaults()
	var out *rag.QueryOutput
	var err error
	if ids := scopedBaseIDs(config); len(ids) > 0 {
		out, err = t.rag.Query(ctx, rag.QueryInput{
			KnowledgeBaseIDs: ids, Query: query, TopK: cfg.MaxChunks, MinScore: float32(cfg.MinScore), IncludeMetadata: true,
			NumCandidates: cfg.NumCandidates, ContextWindow: cfg.ContextWindow, MaxChunksPerDoc: cfg.MaxChunksPerDoc, MaxCharacters: cfg.MaxCharacters,
		})
	} else {
		out, err = t.rag.QueryForAgent(ctx, rag.AgentQueryInput{
			AgentID: agentID, Query: query, TopK: cfg.MaxChunks, MinScore: float32(cfg.MinScore), IncludeMetadata: true,
			NumCandidates: cfg.NumCandidates, ContextWindow: cfg.ContextWindow, MaxChunksPerDoc: cfg.MaxChunksPerDoc, MaxCharacters: cfg.MaxCharacters,
		})
	}
	if err != nil {
		log.Printf("[SearchKnowledgeBase] agent=%s query_chars=%d err=%v", agentID, len([]rune(query)), err)
		return errResult("Erro ao consultar a base de conhecimento. Tente novamente."), nil
	}

	if len(out.Results) == 0 {
		return tools.ExecutionResult{
			Result: fmt.Sprintf("Nenhum resultado para %q. NÃO repita a mesma consulta: reformule com sinônimos ou termos-chave diferentes (mais amplos ou mais específicos, no vocabulário que os documentos da empresa usariam) e busque de novo. Só depois de 2-3 tentativas sem nada relevante, responda com base nas suas próprias instruções e no contexto da conversa — e não invente dados específicos (valores, preços, prazos) que não estejam disponíveis em nenhuma fonte.", query),
		}, nil
	}

	return tools.ExecutionResult{Result: formatSearchResults(query, out.Results)}, nil
}

func formatSearchResults(query string, results []rag.QueryResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Encontrados %d trechos para %q (ordenados por relevância):\n\n", len(results), query))
	for i, r := range results {
		if r.DocumentName != "" {
			sb.WriteString(fmt.Sprintf("[%d] Fonte: %s (relevância: %.2f)\n%s\n\n", i+1, r.DocumentName, r.Score, r.Content))
		} else {
			sb.WriteString(fmt.Sprintf("[%d] (relevância: %.2f)\n%s\n\n", i+1, r.Score, r.Content))
		}
	}
	sb.WriteString("Baseie sua resposta nesses trechos para dados específicos (valores, preços, datas, condições) e não invente o que não estiver neles.")
	return sb.String()
}

var _ tools.Handler = (*searchKnowledgeBaseTool)(nil)
