package tools_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agenttool"
	"vozko/domain/rag"
	"vozko/domain/tools"
)

type scopedRAG struct {
	rag.RAGService
	kbQuery    *rag.QueryInput
	agentQuery *rag.AgentQueryInput
}

func (s *scopedRAG) Query(_ context.Context, in rag.QueryInput) (*rag.QueryOutput, error) {
	s.kbQuery = &in
	return &rag.QueryOutput{Results: []rag.QueryResult{{Content: "catálogo", Score: 0.9}}}, nil
}

func (s *scopedRAG) QueryForAgent(_ context.Context, in rag.AgentQueryInput) (*rag.QueryOutput, error) {
	s.agentQuery = &in
	return &rag.QueryOutput{}, nil
}

type kbBook struct {
	owners map[string]string
	err    error
}

func (b kbBook) FindByIDs(_ context.Context, ids []string) ([]*rag.KnowledgeBase, error) {
	if b.err != nil {
		return nil, b.err
	}
	var out []*rag.KnowledgeBase
	for _, id := range ids {
		if ws, ok := b.owners[id]; ok {
			out = append(out, &rag.KnowledgeBase{ID: id, WorkspaceID: ws})
		}
	}
	return out, nil
}

func TestASearchInstanceScopedToItsBasesQueriesOnlyThem(t *testing.T) {
	r := &scopedRAG{}
	tool := NewSearchKnowledgeBaseToolUseCase(r, kbBook{})
	if tool.Definition().Instances != tools.Many() {
		t.Fatal("knowledge base search allows many scoped instances")
	}
	res, err := tool.ExecuteWithConfig(context.Background(), map[string]interface{}{"__agent_id": "ag-1", "knowledge_base_ids": []interface{}{"kb-catalogo"}}, map[string]interface{}{"query": "preço"})
	if err != nil || res.IsError || r.kbQuery == nil || len(r.kbQuery.KnowledgeBaseIDs) != 1 || r.agentQuery != nil {
		t.Fatalf("res %+v kb %+v agent %+v", res, r.kbQuery, r.agentQuery)
	}
	unscoped := &scopedRAG{}
	_, _ = NewSearchKnowledgeBaseToolUseCase(unscoped, kbBook{}).ExecuteWithConfig(context.Background(), map[string]interface{}{"__agent_id": "ag-1"}, map[string]interface{}{"query": "preço"})
	if unscoped.agentQuery == nil || unscoped.kbQuery != nil {
		t.Fatal("without bases the agent's bases are searched")
	}
}

func TestScopedBasesMustBelongToTheWorkspace(t *testing.T) {
	v := NewSearchKnowledgeBaseToolUseCase(&scopedRAG{}, kbBook{owners: map[string]string{"kb-1": "ws-1", "kb-x": "ws-2"}}).(tools.BindingValidator)
	if err := v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"knowledge_base_ids": []interface{}{"kb-1"}}); err != nil {
		t.Fatalf("own base: %v", err)
	}
	problem, ok := tools.ProblemOf(v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"knowledge_base_ids": []interface{}{"kb-1", "kb-x"}}))
	if !ok || problem.Code != agenttool.CodeKnowledgeBaseInvalid {
		t.Fatalf("problem %+v", problem)
	}
	v = NewSearchKnowledgeBaseToolUseCase(&scopedRAG{}, kbBook{err: errors.New("db down")}).(tools.BindingValidator)
	if _, ok := tools.ProblemOf(v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"knowledge_base_ids": []interface{}{"kb-1"}})); ok {
		t.Fatal("an outage is not a binding problem")
	}
}
