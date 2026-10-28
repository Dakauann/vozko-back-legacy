package automationtransfer_usecase

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/campaign"
	"vozko/domain/conversationvars"
)

func needs(names ...string) automationneeds.Needs {
	out := automationneeds.Needs{}
	for _, name := range names {
		out = append(out, automationneeds.Need{Name: name, Description: "A " + name + " do aluno"})
	}
	return out
}

func mappedConfig(variables ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(variables))
	for _, v := range variables {
		list = append(list, v)
	}
	return destinationsOf(map[string]interface{}{"id": "matricula", "kind": "workflow", "automation_id": "wf-matricula", "stage_id": "st-centro", "variables": list})
}

func (h *harness) mappedRequest(provided map[string]string, variables ...map[string]interface{}) Request {
	req := h.request("matricula", "Quer se matricular.")
	req.Config = mappedConfig(variables...)
	req.Provided = provided
	return req
}

var (
	turmaFromAI    = map[string]interface{}{"name": "turma", "source": "ai", "description": "A turma escolhida"}
	cursoFromLead  = map[string]interface{}{"name": "curso", "source": "lead.custom:curso"}
	poloFixed      = map[string]interface{}{"name": "polo", "source": "literal", "value": "Centro"}
	matriculaAsNow = map[string]interface{}{"name": "matricula", "source": "conversation"}
)

func TestTheTransferStoresTheMappedValuesThenDelegates(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("turma", "curso", "polo")
	h.resolver.lead["lead.custom:curso"] = "Direito"

	done, err := h.service.Transfer(t.Context(), h.mappedRequest(map[string]string{"turma": "Noite"}, turmaFromAI, cursoFromLead, poloFixed))

	require.NoError(t, err)
	assert.Equal(t, []string{"move:st-centro", "store", "delegate"}, h.order)
	assert.Equal(t, []map[string]string{{"turma": "Noite", "curso": "Direito", "polo": "Centro"}}, h.store.saves)
	assert.Equal(t, []string{"ai:ag-prospeccao"}, h.store.by)
	assert.Equal(t, map[string]string{"turma": "Noite", "curso": "Direito", "polo": "Centro"}, h.delegations.delegated[0].Pending)
	assert.Equal(t, []automationtransfer.Filled{{Name: "turma", Source: automationtransfer.SourceAI}, {Name: "curso", Source: "lead.custom:curso"}, {Name: "polo", Source: campaign.BindLiteral}}, done.Record.Filled)
	assert.Equal(t, "turma=ai,curso=lead.custom:curso,polo=literal", h.timeline.events[0].DetailsMap()["filled_variables"])
	assert.NotContains(t, h.timeline.events[0].Details, "Direito")
	assert.NotContains(t, h.timeline.events[0].Details, "Noite")
}

func TestTheCurrentValueIsCheckedButNotRewritten(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("matricula")
	h.resolver.current["matricula"] = "123"

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(nil, matriculaAsNow))

	require.NoError(t, err)
	assert.Equal(t, []string{"move:st-centro", "delegate"}, h.order)
	assert.Empty(t, h.store.saves)
}

func TestAVariableTheModelDidNotInformRefusesBeforeAnythingChanges(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("turma")

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(map[string]string{"turma": " "}, turmaFromAI))

	var named *conversationvars.VariablesError
	require.ErrorIs(t, err, automationtransfer.ErrModelValueMissing)
	require.True(t, errors.As(err, &named))
	assert.Equal(t, []string{"turma"}, named.Names)
	assert.Empty(t, h.order)
	assert.Empty(t, h.delegations.checked)
}

func TestAnEmptyResolvedValueRefusesAndStoresNothing(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("curso")

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(nil, cursoFromLead))

	var named *conversationvars.VariablesError
	require.ErrorIs(t, err, conversationvars.ErrValueEmpty)
	require.True(t, errors.As(err, &named))
	assert.Equal(t, []string{"curso"}, named.Names)
	assert.Empty(t, h.order)
	assert.Empty(t, h.store.saves)
}

func TestAFailedDelegationPutsTheVariablesBack(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("turma", "polo")
	h.store.values["turma"] = "Manhã"
	h.delegations.delegateErr = errors.New("database unavailable")

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(map[string]string{"turma": "Noite"}, turmaFromAI, poloFixed))

	require.Error(t, err)
	assert.Equal(t, []string{"move:st-centro", "store", "delegate", "store", "forget", "remove:st-centro"}, h.order)
	assert.Equal(t, map[string]string{"turma": "Manhã"}, h.store.values)
	assert.Equal(t, [][]string{{"polo"}}, h.store.forgot)
}

func TestAFailedStoreLeavesNoStageChange(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("polo")
	h.store.saveErr = errors.New("connection reset")

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(nil, poloFixed))

	require.Error(t, err)
	assert.Equal(t, []string{"move:st-centro", "store", "remove:st-centro"}, h.order)
	assert.Empty(t, h.delegations.delegated)
}

func TestAVariableNoLongerNeededIsLeftOutAtRuntime(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs()

	_, err := h.service.Transfer(t.Context(), h.mappedRequest(nil, turmaFromAI))

	require.NoError(t, err)
	assert.Empty(t, h.store.saves)
}

func TestTheModelSeesWhatItMustInformForEachDestination(t *testing.T) {
	h := newHarness()
	h.needs["wf-matricula"] = needs("turma", "turno")
	config := mappedConfig(turmaFromAI, map[string]interface{}{"name": "turno", "source": "ai"})

	offers, err := h.service.Offer("ws-1", "ag-prospeccao", config)

	require.NoError(t, err)
	require.Len(t, offers, 1)
	assert.Equal(t, []automationtransfer.VariableSource{
		{Name: "turma", Source: automationtransfer.SourceAI, Description: "A turma escolhida"},
		{Name: "turno", Source: automationtransfer.SourceAI, Description: "A turno do aluno"},
	}, offers[0].Informs)
}

func TestADestinationWhoseNeedsCannotBeReadIsNotOffered(t *testing.T) {
	h := newHarness()
	config := destinationsOf(map[string]interface{}{"id": "x", "kind": "agent", "automation_id": "needs-broken", "variables": []interface{}{turmaFromAI}})
	h.service.d.Agents = agentBook{"needs-broken": {ID: "needs-broken", WorkspaceID: "ws-1", Name: "Quebrado", IsActive: true}}

	offers, err := h.service.Offer("ws-1", "ag-prospeccao", config)

	require.NoError(t, err)
	assert.Empty(t, offers)
}

func TestTheBindingCheckComparesTheMappingWithTheCurrentNeeds(t *testing.T) {
	cases := map[string]struct {
		needs     automationneeds.Needs
		variables []map[string]interface{}
		want      error
		names     []string
	}{
		"matching":         {needs("turma", "curso"), []map[string]interface{}{turmaFromAI, cursoFromLead}, nil, nil},
		"a new need":       {needs("turma", "curso"), []map[string]interface{}{turmaFromAI}, automationtransfer.ErrVariableUnmapped, []string{"curso"}},
		"no longer needed": {needs(), []map[string]interface{}{turmaFromAI}, automationtransfer.ErrVariableNotNeeded, []string{"turma"}},
		"bad source":       {needs("turma"), []map[string]interface{}{{"name": "turma", "source": "lead.custom:sumiu"}}, automationtransfer.ErrVariableSource, []string{"turma"}},
	}
	for name, c := range cases {
		h := newHarness()
		h.needs["wf-matricula"] = c.needs
		err := h.service.CheckBinding("ws-1", "ag-prospeccao", mappedConfig(c.variables...))
		if c.want == nil {
			assert.NoError(t, err, name)
			continue
		}
		assert.ErrorIs(t, err, c.want, name)
		assert.True(t, Problem(err), name)
		var named *conversationvars.VariablesError
		if assert.True(t, errors.As(err, &named), name) {
			assert.True(t, reflect.DeepEqual(named.Names, c.names), "%s: %v", name, named.Names)
		}
	}
}

func TestUnreadableNeedsAreNoBindingProblem(t *testing.T) {
	h := newHarness()
	h.service.d.Agents = agentBook{"needs-broken": {ID: "needs-broken", WorkspaceID: "ws-1", Name: "Quebrado", IsActive: true}}
	err := h.service.CheckBinding("ws-1", "ag-prospeccao", destinationsOf(map[string]interface{}{"id": "x", "kind": "agent", "automation_id": "needs-broken"}))
	require.Error(t, err)
	assert.False(t, Problem(err))
}
