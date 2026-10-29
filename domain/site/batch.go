package site

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"vozko/domain/inlinemarkup"
	"vozko/domain/shared"
	"vozko/domain/sitehtml"
	"vozko/domain/strictjson"
)

const (
	CodeOperationInvalid     = "operation_invalid"
	CodeOperationUnknown     = "operation_unknown"
	CodeFieldRequired        = "field_required"
	CodeFieldInvalid         = "field_invalid"
	CodePathNotFound         = "path_not_found"
	CodePathExists           = "path_exists"
	CodeNodeInvalid          = "node_invalid"
	CodeNodeStale            = "node_stale"
	CodeRevInvalid           = "rev_invalid"
	CodeRefInvalid           = "ref_invalid"
	CodeRefDuplicate         = "ref_duplicate"
	CodeRefUnknown           = "ref_unknown"
	CodePositionInvalid      = "position_invalid"
	CodeTextMarkupInvalid    = "text_markup_invalid"
	CodeAttributeForbidden   = "attribute_forbidden"
	CodeClassInvalid         = "class_invalid"
	CodeBreakpointInvalid    = "breakpoint_invalid"
	CodeMediaInvalid         = "media_invalid"
	CodeLibraryKeyInvalid    = "library_key_invalid"
	CodeTokenInvalid         = "token_invalid"
	CodeKnobUnknown          = "knob_unknown"
	CodeKnobValueInvalid     = "knob_value_invalid"
	CodeEditNoop             = "edit_noop"
	CodeEditPlaceholder      = "edit_placeholder"
	CodeEditNotFound         = "edit_not_found"
	CodeEditAmbiguous        = "edit_ambiguous"
	CodeSkipped              = "skipped"
	CodeOperationFailed      = "operation_failed"
	CodeCopyReferenceMissing = "copy_reference_missing"

	CodeFileStale             = "file_stale"
	CodeKnobExists            = "knob_exists"
	CodeWhatsAppTextTooLong   = "whatsapp_text_too_long"
	CodeWhatsAppNumberUnknown = "whatsapp_number_unknown"

	MaxWhatsAppTextRunes = 500
	WhatsAppNumberAttr   = "data-vz-whatsapp"
	WhatsAppTextAttr     = "data-vz-text"
)

var (
	ErrBatchMalformed = errors.New("site: the edit batch is not a list of operations")
	ErrEditRefused    = errors.New("site: the edit batch was refused")
)

type Failure struct {
	Index  int    `json:"index"`
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type Skip struct {
	Index int `json:"index"`
	After int `json:"after"`
}

type BatchError struct {
	Failures []Failure `json:"failures"`
	Skipped  []Skip    `json:"skipped,omitempty"`
}

func (e *BatchError) Error() string {
	parts := make([]string, 0, len(e.Failures))
	for _, f := range e.Failures {
		part := "operation " + strconv.Itoa(f.Index+1) + ": " + f.Code
		if f.Field != "" {
			part += " (" + f.Field + ")"
		}
		parts = append(parts, part)
	}
	return ErrEditRefused.Error() + ": " + strings.Join(parts, "; ")
}

func (e *BatchError) Is(target error) bool {
	return target == ErrEditRefused
}

type Origin struct {
	Index int
	Op    OperationKind
}

type Batch struct {
	Operations []Operation  `json:"operations"`
	Ignored    []Ignored    `json:"ignored,omitempty"`
	Origins    []Origin     `json:"-"`
	Elements   []ElementUse `json:"-"`
}

func (b Batch) Origin(index int) Origin {
	if index >= 0 && index < len(b.Origins) {
		return b.Origins[index]
	}
	return Origin{Index: index}
}

func (b *Batch) add(index int, kind OperationKind, o Operation) {
	b.Operations = append(b.Operations, o)
	b.Origins = append(b.Origins, Origin{Index: index, Op: kind})
}

var (
	refName       = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
	attrName      = regexp.MustCompile(`^[a-z_:][a-z0-9_.:-]*$`)
	mediaID       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	libraryKey    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	breakpoints   = map[string]bool{"sm": true, "md": true, "lg": true, "xl": true}
	prefixedClass = regexp.MustCompile(`^(sm|md|lg|xl|2xl):`)
	numberRef     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

const (
	refPrefix        = "@"
	maxClassLength   = 200
	maxTokenRunes    = 200
	reservedAttrNode = sitehtml.AnnotationAttr
	reservedAttrItem = "data-vz-i"
)

func Breakpoints() []string {
	return []string{"sm", "md", "lg", "xl"}
}

func PrepareBatch(p Project, raw []byte) (Batch, error) {
	var raws []json.RawMessage
	if strictjson.Decode(raw, &raws) != nil || len(raws) == 0 {
		return Batch{}, ErrBatchMalformed
	}
	s := newBatchState(p)
	batch := Batch{Operations: make([]Operation, 0, len(raws))}
	refused := &BatchError{}
	for i, rawOp := range raws {
		o, ignored, failure := decodeOperation(i, rawOp)
		batch.Ignored = append(batch.Ignored, ignored...)
		keys := o.keys()
		if root, skip := s.poisonedBy(keys); skip {
			batch.add(i, o.Op, o)
			refused.Skipped = append(refused.Skipped, Skip{Index: i, After: root})
			s.poison(keys, root)
			continue
		}
		var expanded []Operation
		if failure == nil {
			expanded, failure = s.expand(i, o)
		}
		if failure == nil {
			failure = s.apply(i, expanded)
		}
		if failure != nil {
			batch.add(i, o.Op, o)
			refused.Failures = append(refused.Failures, *failure)
			s.poison(keys, i)
			for _, e := range expanded {
				s.poison(e.keys(), i)
			}
			continue
		}
		for _, e := range expanded {
			batch.add(i, o.Op, e)
		}
		if o.Op == OpInsertElement {
			batch.Elements = append(batch.Elements, ElementUse{Index: i, Element: o.Element, Key: o.Key})
		}
	}
	if len(refused.Failures) > 0 {
		return batch, refused
	}
	return batch, nil
}

func (s *batchState) apply(i int, ops []Operation) *Failure {
	for _, o := range ops {
		if f := s.validate(i, o); f != nil {
			return f
		}
		s.commit(o)
	}
	return nil
}

func (o Operation) keys() []string {
	var keys []string
	add := func(prefix, value string) {
		if value != "" {
			keys = append(keys, prefix+value)
		}
	}
	nodeKey := func(node string) {
		if name, ok := strings.CutPrefix(node, refPrefix); ok {
			add("ref:", name)
			return
		}
		if file, _, err := sitehtml.ParseNodeID(node); err == nil {
			add("file:", file)
		}
	}
	switch o.Op.Family() {
	case FamilyCode:
		add("file:", o.Path)
		add("file:", o.To)
	case FamilyVisual:
		nodeKey(o.Node)
		nodeKey(o.Before)
		nodeKey(o.After)
		nodeKey(o.Into)
		nodeKey(o.Source)
		if o.At != nil {
			nodeKey(o.At.Before)
			nodeKey(o.At.After)
			nodeKey(o.At.Into)
			add("file:", o.At.Page)
		}
		add("ref:", o.Ref)
	case FamilyTheme:
		if o.Op == OpSetKnob {
			add("knob:", o.Name)
			add("file:", KnobsPath)
		} else {
			add("token:", o.Name)
		}
	case FamilyShorthand:
		if o.Op == OpSetWhatsAppLink || o.Op == OpClearWhatsAppLink || o.Op == OpSetFormField {
			nodeKey(o.Node)
			break
		}
		if o.Op == OpInsertElement || o.Op == OpInsertFormField {
			if o.At != nil {
				nodeKey(o.At.Before)
				nodeKey(o.At.After)
				nodeKey(o.At.Into)
				add("file:", o.At.Page)
			}
			add("ref:", o.Ref)
			break
		}
		add("knob:", o.Name)
		add("knob:", o.To)
		add("file:", KnobsPath)
	}
	return keys
}

type batchState struct {
	p          Project
	files      map[string]bool
	revs       map[string]uint64
	refs       map[string]string
	knobs      *Knobs
	knobsKnown bool
	poisoned   map[string]int
}

func newBatchState(p Project) *batchState {
	s := &batchState{p: p, files: map[string]bool{}, revs: map[string]uint64{}, refs: map[string]string{}, poisoned: map[string]int{}}
	for path, file := range p.Files {
		s.files[path] = true
		s.revs[path] = file.Rev
	}
	s.loadKnobs(p.Content(KnobsPath))
	return s
}

func (s *batchState) loadKnobs(content string, present bool) {
	if !present {
		s.knobs, s.knobsKnown = &Knobs{}, true
		return
	}
	knobs, err := DecodeKnobs([]byte(content))
	if err != nil {
		s.knobs, s.knobsKnown = nil, false
		return
	}
	s.knobs, s.knobsKnown = &knobs, true
}

func (s *batchState) poisonedBy(keys []string) (int, bool) {
	for _, key := range keys {
		if root, ok := s.poisoned[key]; ok {
			return root, true
		}
	}
	return 0, false
}

func (s *batchState) poison(keys []string, root int) {
	for _, key := range keys {
		if _, ok := s.poisoned[key]; !ok {
			s.poisoned[key] = root
		}
	}
}

func (s *batchState) required(path string) bool {
	if slices.Contains(requiredPaths(s.p.Stack), path) {
		return true
	}
	return s.p.Stack == StackReact && (path == ReactEntryJSX || path == ReactEntryTSX)
}

func failure(index int, code, field string) *Failure {
	return &Failure{Index: index, Code: code, Field: field}
}

func (s *batchState) validate(i int, o Operation) *Failure {
	switch o.Op.Family() {
	case FamilyCode:
		return s.validateCode(i, o)
	case FamilyVisual:
		return s.validateVisual(i, o)
	}
	return s.validateTheme(i, o)
}

func (s *batchState) existingPath(i int, path, field string) *Failure {
	if path == "" {
		return failure(i, CodeFieldRequired, field)
	}
	if ValidatePath(s.p.Stack, path) != nil {
		return failure(i, CodePathInvalid, field)
	}
	if !s.files[path] {
		return failure(i, CodePathNotFound, field)
	}
	return nil
}

func (s *batchState) validateCode(i int, o Operation) *Failure {
	switch o.Op {
	case OpWriteFile:
		if o.Path == "" {
			return failure(i, CodeFieldRequired, "path")
		}
		if ValidatePath(s.p.Stack, o.Path) != nil {
			return failure(i, CodePathInvalid, "path")
		}
		if o.Content == nil {
			return failure(i, CodeFieldRequired, "content")
		}
		if len(*o.Content) > MaxProjectBytes {
			return failure(i, CodeFileTooLarge, "content")
		}
		existing, _ := s.p.Content(o.Path)
		if introducesPlaceholder(existing, *o.Content) {
			return failure(i, CodeEditPlaceholder, "content")
		}
		return s.fileRev(i, o)
	case OpEditFile:
		if f := s.existingPath(i, o.Path, "path"); f != nil {
			return f
		}
		if o.Old == "" {
			return failure(i, CodeFieldRequired, "old")
		}
		if o.New == nil {
			return failure(i, CodeFieldRequired, "new")
		}
		if *o.New == o.Old {
			return failure(i, CodeEditNoop, "new")
		}
		if len(*o.New) > MaxProjectBytes {
			return failure(i, CodeFileTooLarge, "new")
		}
		if introducesPlaceholder(o.Old, *o.New) {
			return failure(i, CodeEditPlaceholder, "new")
		}
	case OpDeleteFile:
		if f := s.existingPath(i, o.Path, "path"); f != nil {
			return f
		}
		if s.required(o.Path) {
			return failure(i, CodeFileRequired, "path")
		}
		return s.fileRev(i, o)
	case OpRenameFile:
		if f := s.existingPath(i, o.Path, "path"); f != nil {
			return f
		}
		if s.required(o.Path) {
			return failure(i, CodeFileRequired, "path")
		}
		if o.To == "" {
			return failure(i, CodeFieldRequired, "to")
		}
		if ValidatePath(s.p.Stack, o.To) != nil {
			return failure(i, CodePathInvalid, "to")
		}
		if s.files[o.To] || s.foldedTaken(o.To, o.Path) {
			return failure(i, CodePathExists, "to")
		}
		return s.fileRev(i, o)
	}
	return nil
}

func (s *batchState) fileRev(i int, o Operation) *Failure {
	base, inBase := s.p.Files[o.Path]
	switch {
	case !inBase && o.Rev != 0:
		return failure(i, CodeFileStale, "rev")
	case !inBase:
		return nil
	case o.Rev == 0:
		return failure(i, CodeFieldRequired, "rev")
	case o.Rev > base.Rev:
		return failure(i, CodeRevInvalid, "rev")
	case o.Rev < base.Rev:
		return failure(i, CodeFileStale, "rev")
	}
	return nil
}

func (s *batchState) foldedTaken(path, except string) bool {
	for existing, present := range s.files {
		if present && existing != except && strings.EqualFold(existing, path) {
			return true
		}
	}
	return false
}

type resolvedNode struct {
	file    string
	literal bool
}

func (s *batchState) node(i int, node, field string) (resolvedNode, *Failure) {
	if name, ok := strings.CutPrefix(node, refPrefix); ok {
		file, known := s.refs[name]
		if !known {
			return resolvedNode{}, failure(i, CodeRefUnknown, field)
		}
		return resolvedNode{file: file}, nil
	}
	file, _, err := sitehtml.ParseNodeID(node)
	if err != nil || !MarkupKind(s.p.Stack, file) {
		return resolvedNode{}, failure(i, CodeNodeInvalid, field)
	}
	if !s.files[file] {
		return resolvedNode{}, failure(i, CodePathNotFound, field)
	}
	return resolvedNode{file: file, literal: true}, nil
}

func (s *batchState) checkRev(i int, o Operation, files []resolvedNode) *Failure {
	var literal []string
	for _, f := range files {
		if f.literal {
			literal = append(literal, f.file)
		}
	}
	if len(literal) == 0 {
		return nil
	}
	if o.Rev == 0 {
		return failure(i, CodeFieldRequired, "rev")
	}
	for _, file := range literal {
		if o.Rev > s.revs[file] {
			return failure(i, CodeRevInvalid, "rev")
		}
	}
	return nil
}

func (s *batchState) validateVisual(i int, o Operation) *Failure {
	var touched []resolvedNode
	if o.Op != OpInsertSection {
		if o.Node == "" {
			return failure(i, CodeFieldRequired, "node")
		}
		target, f := s.node(i, o.Node, "node")
		if f != nil {
			return f
		}
		touched = append(touched, target)
		if o.Op == OpMove {
			anchor, field := o.Before, "before"
			if anchors := countSet(o.Before, o.After, o.Into); anchors != 1 {
				return failure(i, CodePositionInvalid, "before")
			}
			if o.After != "" {
				anchor, field = o.After, "after"
			}
			if o.Into != "" {
				anchor, field = o.Into, "into"
			}
			resolved, f := s.node(i, anchor, field)
			if f != nil {
				return f
			}
			if anchor == o.Node || resolved.file != target.file {
				return failure(i, CodePositionInvalid, field)
			}
			touched = append(touched, resolved)
		}
	} else {
		resolved, f := s.placement(i, o.At)
		if f != nil {
			return f
		}
		touched = append(touched, resolved)
	}
	if f := s.checkRev(i, o, touched); f != nil {
		return f
	}
	if o.Op == OpInsertSection && o.Source != "" {
		if f := s.copySource(i, o); f != nil {
			return f
		}
	}
	if o.Ref != "" {
		if !refName.MatchString(o.Ref) {
			return failure(i, CodeRefInvalid, "ref")
		}
		if _, taken := s.refs[o.Ref]; taken {
			return failure(i, CodeRefDuplicate, "ref")
		}
	}
	return s.visualFields(i, o)
}

func countSet(values ...string) int {
	set := 0
	for _, v := range values {
		if v != "" {
			set++
		}
	}
	return set
}

func (s *batchState) copySource(i int, o Operation) *Failure {
	if o.HTML != "" || o.LibraryKey != "" {
		return failure(i, CodeFieldInvalid, "source")
	}
	source, f := s.node(i, o.Source, "source")
	if f != nil {
		return f
	}
	switch {
	case !source.literal && o.SourceRev != 0:
		return failure(i, CodeFieldInvalid, "source_rev")
	case source.literal && o.SourceRev == 0:
		return failure(i, CodeFieldRequired, "source_rev")
	case source.literal && o.SourceRev > s.revs[source.file]:
		return failure(i, CodeRevInvalid, "source_rev")
	}
	return nil
}

func (s *batchState) placement(i int, at *Placement) (resolvedNode, *Failure) {
	if at == nil {
		return resolvedNode{}, failure(i, CodeFieldRequired, "at")
	}
	set := 0
	for _, v := range []string{at.Before, at.After, at.Into, at.Page} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return resolvedNode{}, failure(i, CodePositionInvalid, "at")
	}
	if at.Page != "" {
		if !MarkupKind(s.p.Stack, at.Page) {
			return resolvedNode{}, failure(i, CodePathInvalid, "at")
		}
		if !s.files[at.Page] {
			return resolvedNode{}, failure(i, CodePathNotFound, "at")
		}
		return resolvedNode{file: at.Page, literal: true}, nil
	}
	return s.node(i, at.Before+at.After+at.Into, "at")
}

func (s *batchState) visualFields(i int, o Operation) *Failure {
	switch o.Op {
	case OpSetText:
		if o.Text == nil {
			return failure(i, CodeFieldRequired, "text")
		}
		if strings.TrimSpace(*o.Text) != "" {
			paragraphs, err := inlinemarkup.Parse(*o.Text)
			if err != nil || len(paragraphs) != 1 {
				return failure(i, CodeTextMarkupInvalid, "text")
			}
		}
	case OpSetAttr:
		return s.attribute(i, o)
	case OpSetClasses:
		if len(o.Add) == 0 && len(o.Remove) == 0 {
			return failure(i, CodeFieldRequired, "add")
		}
		if o.Breakpoint != "" && !breakpoints[o.Breakpoint] {
			return failure(i, CodeBreakpointInvalid, "breakpoint")
		}
		for _, list := range []struct {
			field   string
			classes []string
		}{{"add", o.Add}, {"remove", o.Remove}} {
			for _, class := range list.classes {
				if !validClass(class) || (o.Breakpoint != "" && prefixedClass.MatchString(class)) {
					return failure(i, CodeClassInvalid, list.field)
				}
			}
		}
	case OpReplaceImage:
		if o.MediaID == "" {
			return failure(i, CodeFieldRequired, "media_id")
		}
		if !mediaID.MatchString(o.MediaID) {
			return failure(i, CodeMediaInvalid, "media_id")
		}
	case OpInsertSection:
		switch {
		case o.HTML != "" && o.LibraryKey != "":
			return failure(i, CodeFieldInvalid, "library_key")
		case o.Source != "":
		case strings.TrimSpace(o.HTML) == "" && o.LibraryKey == "":
			return failure(i, CodeFieldRequired, "html")
		case o.LibraryKey != "" && !libraryKey.MatchString(o.LibraryKey):
			return failure(i, CodeLibraryKeyInvalid, "library_key")
		case len(o.HTML) > MaxProjectBytes:
			return failure(i, CodeFileTooLarge, "html")
		}
	}
	return nil
}

func validClass(class string) bool {
	return class != "" && len(class) <= maxClassLength && !strings.ContainsAny(class, " \t\r\n\"'`<>{}")
}

func (s *batchState) attribute(i int, o Operation) *Failure {
	name := o.Name
	if name == "" {
		return failure(i, CodeFieldRequired, "name")
	}
	if !attrName.MatchString(name) || strings.HasPrefix(name, "on") || name == reservedAttrNode || name == reservedAttrItem || name == "class" || name == "classname" || strings.EqualFold(name, "srcdoc") {
		return failure(i, CodeAttributeForbidden, "name")
	}
	if o.Value == nil {
		return nil
	}
	value := *o.Value
	switch {
	case urlAttrs[name] && dangerousURL("", value):
		return failure(i, CodeURLForbidden, "value")
	case name == "type" && strings.EqualFold(strings.TrimSpace(value), "password"):
		return failure(i, CodeCredentialFieldForbidden, "value")
	case name == "autocomplete" && credentialTokens(value):
		return failure(i, CodeCredentialFieldForbidden, "value")
	case (name == "action" || name == "formaction") && externalURL(value):
		return failure(i, CodeFormActionForbidden, "value")
	}
	return nil
}

func credentialTokens(value string) bool {
	for _, token := range strings.Fields(strings.ToLower(value)) {
		if strings.HasPrefix(token, "cc-") || credentialAutocomplete[token] {
			return true
		}
	}
	return false
}

func (s *batchState) validateTheme(i int, o Operation) *Failure {
	if o.Name == "" {
		return failure(i, CodeFieldRequired, "name")
	}
	if o.Op == OpSetToken {
		if !cssVariable.MatchString(o.Name) {
			return failure(i, CodeTokenInvalid, "name")
		}
		if o.Value == nil {
			return failure(i, CodeFieldRequired, "value")
		}
		if !validTokenValue(*o.Value) {
			return failure(i, CodeTokenInvalid, "value")
		}
		return nil
	}
	if o.Value == nil && o.Number == nil {
		return failure(i, CodeFieldRequired, "value")
	}
	if !s.knobsKnown {
		return nil
	}
	knob, ok := s.knobs.Find(o.Name)
	if !ok {
		return failure(i, CodeKnobUnknown, "name")
	}
	if _, err := knob.CSSValue(KnobValue{Number: o.Number, Text: o.Value}); err != nil {
		return failure(i, CodeKnobValueInvalid, "value")
	}
	return nil
}

func validTokenValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= maxTokenRunes &&
		!strings.ContainsAny(trimmed, ";{}<>\n\r\\@") && !strings.Contains(lower, "url(") && !strings.Contains(trimmed, "/*")
}

func (s *batchState) bump(path string) {
	s.revs[path]++
	if s.revs[path] == 0 {
		s.revs[path] = 1
	}
}

func (s *batchState) commit(o Operation) {
	switch o.Op {
	case OpWriteFile:
		s.files[o.Path] = true
		s.bump(o.Path)
		if o.Path == KnobsPath {
			s.loadKnobs(*o.Content, true)
		}
	case OpEditFile:
		s.bump(o.Path)
		if o.Path == KnobsPath {
			s.knobs, s.knobsKnown = nil, false
		}
	case OpDeleteFile:
		delete(s.files, o.Path)
		delete(s.revs, o.Path)
		if o.Path == KnobsPath {
			s.loadKnobs("", false)
		}
	case OpRenameFile:
		delete(s.files, o.Path)
		s.files[o.To] = true
		s.revs[o.To] = s.revs[o.Path] + 1
		delete(s.revs, o.Path)
		if o.Path == KnobsPath {
			s.loadKnobs("", false)
		}
		if o.To == KnobsPath {
			s.knobs, s.knobsKnown = nil, false
		}
	default:
		if o.Op.Family() != FamilyVisual {
			return
		}
		file := s.visualFile(o)
		s.bump(file)
		if o.Ref != "" {
			s.refs[o.Ref] = file
		}
	}
}

func (s *batchState) visualFile(o Operation) string {
	node := o.Node
	if o.Op == OpInsertSection {
		if o.At.Page != "" {
			return o.At.Page
		}
		node = o.At.Before + o.At.After + o.At.Into
	}
	if name, ok := strings.CutPrefix(node, refPrefix); ok {
		return s.refs[name]
	}
	file, _, _ := sitehtml.ParseNodeID(node)
	return file
}

var placeholderPhrases = []string{
	"existing code", "rest unchanged", "rest of the code", "rest of the file", "remains unchanged", "unchanged code",
	"previous code", "same as before", "codigo existente", "resto do codigo", "restante do codigo", "resto inalterado",
	"mesmo codigo", "codigo anterior",
}

var commentOpeners = []string{"//", "/*", "{/*", "<!--", "#", "*"}

func placeholderLines(text string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "..." || trimmed == "…" {
			out[trimmed] = true
			continue
		}
		opened := false
		for _, opener := range commentOpeners {
			if strings.HasPrefix(trimmed, opener) {
				opened = true
				break
			}
		}
		if !opened {
			continue
		}
		folded := shared.FoldForMatch(trimmed)
		body := strings.Trim(folded, "/*{}<>!-# .…")
		if body == "" && strings.Contains(folded, "...") {
			out[trimmed] = true
			continue
		}
		for _, phrase := range placeholderPhrases {
			if strings.Contains(folded, phrase) {
				out[trimmed] = true
				break
			}
		}
	}
	return out
}

func introducesPlaceholder(before, after string) bool {
	existing := placeholderLines(before)
	for line := range placeholderLines(after) {
		if !existing[line] {
			return true
		}
	}
	return false
}
