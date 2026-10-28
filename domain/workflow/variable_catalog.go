package workflow

import (
	"sort"
	"strings"

	"vozko/domain/shared"

	"vozko/domain/variabledecl"
)

type VariableGroup string

const (
	VariableGroupConversation VariableGroup = "conversation"
	VariableGroupTrigger      VariableGroup = "trigger"
	VariableGroupNodes        VariableGroup = "nodes"
	VariableGroupContact      VariableGroup = "contact"
	VariableGroupSystem       VariableGroup = "system"
	VariableGroupSecrets      VariableGroup = "secrets"
)

type VariableKind string

const (
	VariableKindConversation VariableKind = "conversation"
	VariableKindDefault      VariableKind = "default"
	VariableKindTrigger      VariableKind = "trigger"
	VariableKindNode         VariableKind = "node"
	VariableKindSystem       VariableKind = "system"
	VariableKindSecret       VariableKind = "secret"
)

type VariableType string

const (
	VariableTypeText    VariableType = "text"
	VariableTypeNumber  VariableType = "number"
	VariableTypeBoolean VariableType = "boolean"
	VariableTypeObject  VariableType = "object"
	VariableTypeList    VariableType = "list"
	VariableTypeDate    VariableType = "date"
	VariableTypeUnknown VariableType = "unknown"
)

type VariableAvailability string

const (
	AvailableAlways      VariableAvailability = "always"
	AvailableOnSomePaths VariableAvailability = "some_paths"
)

var availableNever VariableAvailability

type VariableSource struct {
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName"`
}

type CatalogVariable struct {
	Ref          string               `json:"ref" example:"input.cpf"`
	Label        string               `json:"label" example:"Enviado pelo agente › cpf"`
	LabelKey     string               `json:"labelKey,omitempty" example:"contact_number"`
	LabelDetail  string               `json:"labelDetail,omitempty" example:"cpf"`
	LabelPrefix  string               `json:"labelPrefix,omitempty" example:"Busca cadastro"`
	Open         bool                 `json:"open"`
	Undeclared   bool                 `json:"undeclared,omitempty"`
	Group        VariableGroup        `json:"group" enums:"conversation,trigger,nodes,contact,system,secrets"`
	Kind         VariableKind         `json:"kind" enums:"conversation,default,trigger,node,system,secret"`
	Source       *VariableSource      `json:"source"`
	Type         VariableType         `json:"type" enums:"text,number,boolean,object,list,date,unknown"`
	Example      *string              `json:"example"`
	Availability VariableAvailability `json:"availability" enums:"always,some_paths"`
	Channels     *VariableChannelNote `json:"channels,omitempty"`
}

type VariableCatalog struct {
	Variables []CatalogVariable `json:"variables"`
}

type VariableSources struct {
	Specs        []TriggerInputSpec
	Forms        map[string]TriggerForm
	Secrets      []string
	Declarations []variabledecl.Declaration
}

var variableGroupRank = map[VariableGroup]int{
	VariableGroupConversation: -1,
	VariableGroupTrigger:      0,
	VariableGroupNodes:        1,
	VariableGroupContact:      2,
	VariableGroupSystem:       3,
	VariableGroupSecrets:      4,
}

func VariableCatalogAt(g *Graph, catalog []NodeDefinition, sources VariableSources, nodeID string) VariableCatalog {
	flow := newVariableFlow(g, NodeCatalogMap(catalog), sources)
	return VariableCatalog{Variables: flow.catalogAt(nodeID, sources.Secrets)}
}

type catalogCandidate struct {
	variable CatalogVariable
	distance int
	order    int
	seq      int
}

func (f *variableFlow) catalogAt(nodeID string, secrets []string) []CatalogVariable {
	var candidates []catalogCandidate
	if idx, reachable := f.index[nodeID]; reachable && f.reached[idx] {
		distance := f.graph.UpstreamDistances(f.graph.Nodes[idx].ID)
		for seq, p := range f.provisions {
			if p.hidden || !f.may[idx].has(seq) {
				continue
			}
			source := f.sourceOf(p.node)
			if p.group == VariableGroupConversation {
				source = nil
			}
			if _, upstream := distance[f.graph.Nodes[p.node].ID]; !upstream {
				continue
			}
			candidates = append(candidates, catalogCandidate{
				variable: CatalogVariable{
					Ref: p.ref, Label: p.label, LabelKey: p.labelKey, LabelDetail: p.labelDetail, LabelPrefix: p.labelPrefix, Open: p.match == matchOpen, Undeclared: p.undeclared,
					Group: p.group, Kind: p.kind, Source: source, Type: p.typ, Example: p.example,
					Availability: f.availabilityAt(idx, p.ref),
					Channels:     ChannelNoteOf(p.ref),
				},
				distance: distance[f.graph.Nodes[p.node].ID],
				order:    p.node,
				seq:      seq,
			})
		}
	}
	everyPath := map[int]bool{}
	for _, c := range candidates {
		if c.variable.Availability == AvailableAlways {
			everyPath[c.order] = true
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if ra, rb := variableGroupRank[a.variable.Group], variableGroupRank[b.variable.Group]; ra != rb {
			return ra < rb
		}
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if ea, eb := everyPath[a.order], everyPath[b.order]; ea != eb {
			return ea
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.seq < b.seq
	})
	out := make([]CatalogVariable, 0, len(candidates)+len(systemVariables)+len(secrets))
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c.variable.Ref] {
			continue
		}
		seen[c.variable.Ref] = true
		out = append(out, c.variable)
	}
	for _, v := range systemVariables {
		if key, ok := strings.CutPrefix(v.Ref, "sys."); ok {
			if now, builtin := builtinSys(key); builtin {
				v.Example = &now
			}
		}
		out = append(out, v)
	}
	for _, name := range sortedSecretNames(secrets) {
		out = append(out, CatalogVariable{
			Ref: SecretScope + "." + name, Label: "Segredo › " + name, LabelKey: "secret", LabelDetail: name, Group: VariableGroupSecrets, Kind: VariableKindSecret,
			Type: VariableTypeText, Availability: AvailableAlways,
		})
	}
	return out
}

func sortedSecretNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

var systemVariables = []CatalogVariable{
	{Ref: "sys.date", Label: "Sistema › data de hoje", LabelKey: "sys_date", Group: VariableGroupSystem, Kind: VariableKindDefault, Type: VariableTypeDate, Availability: AvailableAlways},
	{Ref: "sys.time", Label: "Sistema › hora agora", LabelKey: "sys_time", Group: VariableGroupSystem, Kind: VariableKindDefault, Type: VariableTypeText, Availability: AvailableAlways},
	{Ref: "sys.timestamp", Label: "Sistema › timestamp Unix", LabelKey: "sys_timestamp", Group: VariableGroupSystem, Kind: VariableKindDefault, Type: VariableTypeNumber, Availability: AvailableAlways},
}

func (f *variableFlow) sourceOf(idx int) *VariableSource {
	n := &f.graph.Nodes[idx]
	return &VariableSource{NodeID: n.ID, NodeName: f.nodeName(idx)}
}

func (f *variableFlow) nodeName(idx int) string {
	n := &f.graph.Nodes[idx]
	if name := n.DisplayName(); name != "" {
		return name
	}
	if def, ok := f.defs[n.Type]; ok && def.Label != "" {
		return def.Label
	}
	return n.ID
}

func (c VariableCatalog) LabelOf(ref string) (string, bool) {
	ref = canonicalRef(ref)
	var best *CatalogVariable
	for i := range c.Variables {
		v := &c.Variables[i]
		if v.Ref == ref {
			return v.Label, true
		}
		if v.Open && strings.HasPrefix(ref, v.Ref+".") && (best == nil || len(v.Ref) > len(best.Ref)) {
			best = v
		}
	}
	if best == nil {
		return "", false
	}
	return label(append([]string{best.Label}, strings.Split(strings.TrimPrefix(ref, best.Ref+"."), ".")...)...), true
}

func searchText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(shared.FoldAccents(text))), " ")
}

func (c VariableCatalog) Search(query string) VariableCatalog {
	needle := searchText(query)
	if needle == "" {
		return c
	}
	tiers := make([][]CatalogVariable, 3)
	for _, v := range c.Variables {
		label := searchText(v.Label)
		switch {
		case label == needle:
			tiers[0] = append(tiers[0], v)
		case strings.HasPrefix(label, needle):
			tiers[1] = append(tiers[1], v)
		case strings.Contains(label, needle) || strings.Contains(strings.ToLower(v.Ref), needle):
			tiers[2] = append(tiers[2], v)
		}
	}
	out := make([]CatalogVariable, 0, len(tiers[0])+len(tiers[1])+len(tiers[2]))
	for _, tier := range tiers {
		out = append(out, tier...)
	}
	return VariableCatalog{Variables: out}
}

type RefLabel struct {
	Label       string `json:"label,omitempty" example:"Busca cadastro › Código HTTP"`
	LabelKey    string `json:"labelKey,omitempty" example:"output_status_code"`
	LabelPrefix string `json:"labelPrefix,omitempty" example:"Busca cadastro"`
	LabelDetail string `json:"labelDetail,omitempty" example:"status"`
	Unknown     bool   `json:"unknown,omitempty"`
}

func (c VariableCatalog) RefLabelOf(ref string) RefLabel {
	ref = canonicalRef(ref)
	var best *CatalogVariable
	for i := range c.Variables {
		v := &c.Variables[i]
		if v.Ref == ref {
			return RefLabel{Label: v.Label, LabelKey: v.LabelKey, LabelPrefix: v.LabelPrefix, LabelDetail: v.LabelDetail}
		}
		if v.Open && strings.HasPrefix(ref, v.Ref+".") && (best == nil || len(v.Ref) > len(best.Ref)) {
			best = v
		}
	}
	if best == nil {
		return RefLabel{Unknown: true}
	}
	rest := strings.Split(strings.TrimPrefix(ref, best.Ref+"."), ".")
	detail := label(rest...)
	if best.LabelDetail != "" {
		detail = label(append([]string{best.LabelDetail}, rest...)...)
	}
	return RefLabel{Label: label(append([]string{best.Label}, rest...)...), LabelKey: best.LabelKey, LabelPrefix: best.LabelPrefix, LabelDetail: detail}
}

func ReferenceLabels(g *Graph, catalog []NodeDefinition, sources VariableSources) map[string]map[string]RefLabel {
	flow := newVariableFlow(g, NodeCatalogMap(catalog), sources)
	out := map[string]map[string]RefLabel{}
	for _, node := range g.Nodes {
		refs := templateRefsIn(node.Config)
		if len(refs) == 0 {
			continue
		}
		at := VariableCatalog{Variables: flow.catalogAt(node.ID, sources.Secrets)}
		labels := make(map[string]RefLabel, len(refs))
		for _, ref := range refs {
			labels[ref] = at.RefLabelOf(ref)
		}
		out[node.ID] = labels
	}
	return out
}

func templateRefsIn(v interface{}) []string {
	seen := map[string]bool{}
	var refs []string
	var walk func(interface{})
	walk = func(value interface{}) {
		switch val := value.(type) {
		case string:
			for _, segment := range TemplateSegments(val) {
				if segment.Ref != "" && !seen[segment.Ref] {
					seen[segment.Ref] = true
					refs = append(refs, segment.Ref)
				}
			}
		case map[string]interface{}:
			for _, item := range val {
				walk(item)
			}
		case []interface{}:
			for _, item := range val {
				walk(item)
			}
		}
	}
	walk(v)
	return refs
}
