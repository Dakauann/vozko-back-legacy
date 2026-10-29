package campaignaudience

import (
	"strings"

	"vozko/domain/campaign"
	"vozko/domain/variabledecl"
)

type Outcome string

const (
	OutcomeSend      Outcome = "send"
	OutcomeOut       Outcome = "out"
	OutcomeUndecided Outcome = "undecided"
)

type Blocker string

const (
	BlockerChoiceRequired           Blocker = "variable_choice_required"
	BlockerSourceRequired           Blocker = "variable_source_required"
	BlockerListVariablesMissing     Blocker = "list_variables_missing"
	BlockerListMetadataMissing      Blocker = "list_metadata_missing"
	BlockerListColumnMissing        Blocker = "list_column_missing"
	BlockerLeadsMetadataUnavailable Blocker = "leads_metadata_unavailable"
	BlockerTemplateRequired         Blocker = "template_required"
	BlockerMessageRequired          Blocker = "message_required"
	BlockerSheetRequired            Blocker = "sheet_required"
	BlockerSheetUnfit               Blocker = "sheet_unfit"
)

const ReasonMissingVariable = string(campaign.SkipMissingVariable)

type Candidate struct {
	Line         int
	LeadID       string
	Contact      string
	Name         string
	HasContact   bool
	Refusal      string
	Values       []string
	Metadata     map[string]any
	FirstContact bool
}

type Decision struct {
	Outcome         Outcome
	Reason          string
	Values          []string
	Missing         []campaign.MissingVariable
	Empty           []string
	MetadataMissing []string
}

type VariableCount struct {
	Key            string                 `json:"key"`
	Name           string                 `json:"name,omitempty"`
	Header         bool                   `json:"header"`
	Source         campaign.BindingSource `json:"source,omitempty"`
	Value          string                 `json:"value,omitempty"`
	Missing        int                    `json:"missing"`
	Policy         MissingPolicy          `json:"policy"`
	Text           string                 `json:"text,omitempty"`
	Fallback       string                 `json:"fallback,omitempty"`
	Suggested      bool                   `json:"suggested"`
	ChoiceRequired bool                   `json:"choiceRequired"`
	Kind           SlotKind               `json:"kind" enums:"template,automation"`
	NeededBy       string                 `json:"neededBy,omitempty"`
	Column         string                 `json:"column,omitempty" example:"VAR3"`
	ColumnMissing  bool                   `json:"columnMissing"`
	Optional       bool                   `json:"optional"`
	Default        string                 `json:"default,omitempty" example:"cordial"`
	Invalid        int                    `json:"invalid"`
	Sheet          bool                   `json:"sheet"`
	SheetUnfit     string                 `json:"sheetUnfit,omitempty" example:"sheet_fit_tab_missing"`
	Type           *VariableType          `json:"type,omitempty"`
}

type VariableType struct {
	Kind    variabledecl.Kind  `json:"kind" example:"choice"`
	Shape   variabledecl.Shape `json:"shape" enums:"text,number,date,choice,sheet" example:"choice"`
	Options []string           `json:"options"`
}

func typeOf(b Bound) *VariableType {
	if !b.Automation() || b.Type.Kind == "" {
		return nil
	}
	options := b.Type.Options
	if options == nil {
		options = []string{}
	}
	return &VariableType{Kind: b.Type.Kind, Shape: b.Shape, Options: options}
}

type SelectionCount struct {
	Selected     int    `json:"selected"`
	ConfirmCount int    `json:"confirmCount"`
	Fingerprint  string `json:"fingerprint,omitempty"`
}

type ListCount struct {
	ID       string `json:"id"`
	Rows     int    `json:"rows"`
	Existing int    `json:"existing"`
	New      int    `json:"new"`
}

type Count struct {
	Channel           Channel         `json:"channel"`
	Source            Source          `json:"source"`
	Matched           int             `json:"matched"`
	WithContact       int             `json:"withContact"`
	Sendable          *int            `json:"sendable"`
	Out               map[string]int  `json:"out"`
	OutTotal          int             `json:"outTotal"`
	RequiredVariables int             `json:"requiredVariables"`
	Variables         []VariableCount `json:"variables"`
	Blockers          []Blocker       `json:"blockers"`
	Selection         *SelectionCount `json:"selection"`
	List              *ListCount      `json:"list"`
	MetadataMissing   []MetadataGap   `json:"metadataMissing"`
}

type Tally struct {
	channel   Channel
	source    Source
	bound     []Bound
	missing   []int
	matched   int
	contact   int
	sendable  int
	undecided int
	out       map[string]int
	metadata  *MetadataGaps
	awaits    Blocker
	accept    func(key, value string) bool
	invalid   []int
}

func NewTally(channel Channel, source Source, bound []Bound) *Tally {
	return &Tally{channel: channel, source: source, bound: bound, missing: make([]int, len(bound)), invalid: make([]int, len(bound)), out: map[string]int{}, metadata: NewMetadataGaps(nil)}
}

func (t *Tally) Checking(accept func(key, value string) bool) *Tally {
	t.accept = accept
	return t
}

func (t *Tally) fits(i int, value string) bool {
	b := t.bound[i]
	if value == "" || t.accept == nil || !b.Automation() || b.Sheet || t.accept(b.Key, value) {
		return true
	}
	t.invalid[i]++
	return false
}

func (t *Tally) Requiring(rule MetadataRule) *Tally {
	t.metadata = NewMetadataGaps(rule)
	return t
}

func (t *Tally) Awaiting(blocker Blocker) *Tally {
	t.awaits = blocker
	return t
}

func (t *Tally) sourcesRequired() bool {
	return t.source == SourceLeads && t.channel.bindsSources()
}

func (t *Tally) skipsSlot(i int) bool {
	b := t.bound[i]
	return !b.fed() && (b.Optional || b.Sheet || t.sourcesRequired() || b.Source == campaign.BindLiteral)
}

func (t *Tally) Add(c Candidate) Decision {
	t.matched++
	if c.HasContact {
		t.contact++
	}
	var d Decision
	if t.source == SourceList {
		d = t.addRow(c)
	} else {
		d = t.addLead(c)
	}
	d.MetadataMissing = t.metadata.Add(c.Line, c.Metadata)
	return d
}

func (t *Tally) lack(i int, empty []string) []string {
	t.missing[i]++
	return append(empty, t.bound[i].Key)
}

func (t *Tally) addLead(c Candidate) Decision {
	if c.Refusal != "" {
		t.out[c.Refusal]++
		return Decision{Outcome: OutcomeOut, Reason: c.Refusal}
	}
	values, r := t.resolve(c, func(Bound) bool { return true })
	return t.decide(values, r)
}

type resolution struct {
	empty     []string
	excluded  []campaign.MissingVariable
	undecided bool
}

func (t *Tally) resolve(c Candidate, choosable func(Bound) bool) ([]string, resolution) {
	values := make([]string, len(t.bound))
	var r resolution
	for i, b := range t.bound {
		if i < len(c.Values) {
			values[i] = strings.TrimSpace(c.Values[i])
		}
		if !t.fits(i, values[i]) {
			values[i] = ""
		}
		if values[i] != "" || t.skipsSlot(i) {
			continue
		}
		r.empty = t.lack(i, r.empty)
		if b.Optional || !choosable(b) {
			continue
		}
		switch t.policyOf(b) {
		case MissingText:
			values[i] = t.textOf(b)
		case MissingExclude:
			r.excluded = append(r.excluded, campaign.MissingVariable{Slot: i + 1, Source: b.Source})
		default:
			r.undecided = true
		}
	}
	return values, r
}

func (t *Tally) decide(values []string, r resolution) Decision {
	switch {
	case len(r.excluded) > 0:
		t.out[ReasonMissingVariable]++
		return Decision{Outcome: OutcomeOut, Reason: ReasonMissingVariable, Missing: r.excluded, Empty: r.empty}
	case r.undecided:
		t.undecided++
		return Decision{Outcome: OutcomeUndecided, Values: values, Empty: r.empty}
	}
	t.sendable++
	return Decision{Outcome: OutcomeSend, Values: values, Empty: r.empty}
}

func (t *Tally) policyOf(b Bound) MissingPolicy {
	if b.Missing == "" && !t.channel.RequiresChoice() {
		return MissingText
	}
	return b.Missing
}

func (t *Tally) textOf(b Bound) string {
	if b.Missing == MissingText {
		return b.Text
	}
	return b.Fallback
}

func (t *Tally) blockers() []Blocker {
	blockers := []Blocker{}
	if t.awaits != "" {
		blockers = append(blockers, t.awaits)
	}
	for _, b := range t.bound {
		if b.ColumnMissing {
			blockers = append(blockers, BlockerListColumnMissing)
			break
		}
	}
	if blocker, _ := t.sheetBlocker(); blocker != "" {
		blockers = append(blockers, blocker)
	}
	for i, b := range t.bound {
		if t.skipsSlot(i) && !b.Optional && !b.Sheet {
			blockers = append(blockers, BlockerSourceRequired)
			break
		}
	}
	if t.undecided > 0 {
		blockers = append(blockers, BlockerChoiceRequired)
	}
	if t.source == SourceList {
		for i, n := range t.missing {
			if n > 0 && !t.bound[i].Automation() {
				blockers = append(blockers, BlockerListVariablesMissing)
				break
			}
		}
	}
	if t.metadata.Any() {
		blockers = append(blockers, t.metadataBlocker())
	}
	return blockers
}

func (t *Tally) metadataBlocker() Blocker {
	if t.source == SourceList {
		return BlockerListMetadataMissing
	}
	return BlockerLeadsMetadataUnavailable
}

func (t *Tally) sheetBlocker() (Blocker, string) {
	for _, b := range t.bound {
		switch {
		case !b.Sheet || b.Optional && Unbound(b):
		case Unbound(b):
			return BlockerSheetRequired, b.Key
		case b.SheetUnfit != "":
			return BlockerSheetUnfit, b.Key
		}
	}
	return "", ""
}

func (t *Tally) Creatable() error {
	for _, blocker := range t.blockers() {
		switch blocker {
		case BlockerSheetRequired:
			_, key := t.sheetBlocker()
			return variableError(ErrSheetRequired, key)
		case BlockerSheetUnfit:
			_, key := t.sheetBlocker()
			return variableError(ErrSheetUnfit, key)
		case BlockerTemplateRequired:
			return ErrTemplateRequired
		case BlockerMessageRequired:
			return ErrMessageRequired
		case BlockerListColumnMissing:
			return ErrListColumnUnknown
		case BlockerSourceRequired:
			return ErrSourceRequired
		case BlockerChoiceRequired:
			return ErrChoiceRequired
		case BlockerListVariablesMissing:
			return ErrListVariablesMissing
		case BlockerListMetadataMissing:
			return ErrListMetadataMissing
		case BlockerLeadsMetadataUnavailable:
			return ErrLeadsMetadataUnavailable
		}
	}
	return nil
}

func (t *Tally) Count() Count {
	count := Count{
		Channel: t.channel, Source: t.source, Matched: t.matched, WithContact: t.contact,
		Out: map[string]int{}, RequiredVariables: t.templateSlots(), Variables: []VariableCount{}, Blockers: t.blockers(), MetadataMissing: t.metadata.Gaps(),
	}
	for reason, n := range t.out {
		count.Out[reason] = n
		count.OutTotal += n
	}
	for i, b := range t.bound {
		v := VariableCount{Key: b.Key, Name: b.Name, Header: b.Header, Source: b.Source, Value: b.Value, Missing: t.missing[i], Policy: b.Missing, Text: b.Text, Fallback: b.Fallback, Suggested: b.Suggested, Kind: SlotTemplate, NeededBy: b.NeededBy, Column: b.Column, ColumnMissing: b.ColumnMissing, Optional: b.Optional, Default: b.Default, Invalid: t.invalid[i], Sheet: b.Sheet, SheetUnfit: b.SheetUnfit, Type: typeOf(b)}
		if b.Automation() {
			v.Kind = SlotAutomation
		}
		v.ChoiceRequired = (t.source == SourceLeads || b.Automation()) && !b.Optional && t.channel.RequiresChoice() && b.Missing == "" && t.missing[i] > 0
		count.Variables = append(count.Variables, v)
	}
	undecidable := false
	for _, blocker := range count.Blockers {
		undecidable = undecidable || blocker == BlockerChoiceRequired || blocker == BlockerSourceRequired
	}
	if !undecidable {
		sendable := t.sendable
		count.Sendable = &sendable
	}
	return count
}

func (t *Tally) addRow(c Candidate) Decision {
	values, r := t.resolve(c, Bound.Automation)
	if c.Refusal != "" {
		t.out[c.Refusal]++
		return Decision{Outcome: OutcomeOut, Reason: c.Refusal, Values: values, Empty: r.empty}
	}
	return t.decide(values, r)
}
