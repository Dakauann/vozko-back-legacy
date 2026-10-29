package sitetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/strictjson"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/copilot/copilottools/screencall"
	site_usecase "vozko/usecases/site"
)

const (
	editNoOperations = "envie ao menos uma operação em operations"
	editMalformed    = "operations precisa ser uma lista de operações"
	editRefused      = "nada foi aplicado; corrija e reenvie o lote inteiro: "
	editNoBuilder    = "o construtor de sites não está disponível agora, então nada foi aplicado; tente de novo em instantes"
	editMoved        = "o site mudou duas vezes enquanto você editava, então nada foi aplicado; leia de novo com site_read"
	editLabelBad     = "label vai até 120 caracteres, numa linha"
	editNoChange     = "o lote não mudou nenhum arquivo"
	editNotShown     = "o lote está guardado no site, mas o editor não mostrou: "
	opPrefix         = "operação %d (%s): %s"
	opSkipped        = "operação %d pulada: dependia da operação %d"
	opIgnored        = "operação %d: campo %s ignorado, não existe para essa op"
	opDefault        = "a operação não pôde ser aplicada"
	maxEditLabel     = 120
)

var codeWords = map[string]string{
	site.CodeOperationInvalid:      "a operação não é um objeto",
	site.CodeOperationUnknown:      "op não existe",
	site.CodeFieldRequired:         "falta o campo",
	site.CodeFieldInvalid:          "o campo tem o tipo errado",
	site.CodePathNotFound:          "o arquivo não existe; confira o mapa",
	site.CodePathExists:            "já existe um arquivo nesse caminho",
	site.CodePathInvalid:           "caminho fora das pastas permitidas",
	site.CodeNodeInvalid:           "node não está no formato arquivo#offset",
	site.CodeNodeStale:             "o arquivo mudou depois da sua leitura; leia de novo e use node e rev novos",
	site.CodeRevInvalid:            "rev precisa ser o rev lido do arquivo",
	site.CodeRefInvalid:            "ref usa letras minúsculas, números, - e _, até 32",
	site.CodeRefDuplicate:          "esse ref já foi usado neste lote",
	site.CodeRefUnknown:            "o @ref não foi criado antes neste lote",
	site.CodePositionInvalid:       "use um só entre before, after, into e page",
	site.CodeTextMarkupInvalid:     "a marcação do texto está incompleta",
	site.CodeAttributeForbidden:    "esse atributo não pode ser mudado assim",
	site.CodeClassInvalid:          "classe inválida",
	site.CodeBreakpointInvalid:     "breakpoint é sm, md, lg ou xl",
	site.CodeMediaInvalid:          "media_id não é uma imagem da biblioteca",
	site.CodeLibraryKeyInvalid:     "library_key não existe",
	site.CodeCopyReferenceMissing:  "o elemento copiado usa nomes que o arquivo de destino não tem (veja detail)",
	site.CodeTokenInvalid:          "nome ou valor de token inválido",
	site.CodeKnobUnknown:           "esse knob não existe em knobs.json",
	site.CodeKnobValueInvalid:      "valor fora do knob",
	site.CodeEditNoop:              "old e new são iguais",
	site.CodeEditPlaceholder:       "o conteúdo traz um marcador de omissão; escreva o trecho completo",
	site.CodeEditNotFound:          "o old não aparece no arquivo; copie o trecho exato com site_read",
	site.CodeEditAmbiguous:         "o old aparece mais de uma vez; inclua mais contexto ou use replace_all",
	site.CodeFileTooLarge:          "arquivo grande demais",
	site.CodeFileStale:             "o arquivo mudou depois da sua leitura; leia de novo com site_read e use o rev novo",
	site.CodeKnobExists:            "já existe um knob com esse nome ou variável",
	site.CodeKnobsInvalid:          "knobs.json não pode ser lido ou o knob é inválido",
	site.CodeWhatsAppTextTooLong:   "a mensagem do WhatsApp passa de 500 caracteres",
	site.CodeWhatsAppNumberUnknown: "esse número de WhatsApp não é deste workspace; veja as referências do site_read",
	site.CodeOperationFailed:       "o construtor falhou nessa operação; leia o arquivo de novo com site_read e refaça a mudança com edit_file ou write_file",
}

type siteAtArgs struct {
	Before string `json:"before,omitempty" desc:"node: logo antes"`
	After  string `json:"after,omitempty" desc:"node: logo depois"`
	Into   string `json:"into,omitempty" desc:"node: no fim dele"`
	Page   string `json:"page,omitempty" desc:"arquivo de página: no fim do conteúdo"`
}

type siteOperationArgs struct {
	Op         string      `json:"op" req:"true" enum:"write_file,edit_file,delete_file,rename_file,set_text,set_attr,set_classes,replace_image,move,duplicate,remove,insert_section,set_token,set_knob,define_knob,rename_knob,remove_knob,set_whatsapp_link,clear_whatsapp_link" desc:"a operação"`
	Path       string      `json:"path,omitempty" desc:"arquivo"`
	To         string      `json:"to,omitempty" desc:"rename_file: novo caminho"`
	Content    *string     `json:"content,omitempty" desc:"write_file: o arquivo inteiro"`
	Old        string      `json:"old,omitempty" desc:"edit_file: trecho exato e único"`
	New        *string     `json:"new,omitempty" desc:"edit_file: o trecho novo"`
	ReplaceAll bool        `json:"replace_all,omitempty" desc:"edit_file: troca todas as ocorrências"`
	Node       string      `json:"node,omitempty" desc:"arquivo#offset ou @ref"`
	Rev        int         `json:"rev,omitempty" desc:"rev lido do arquivo: com node e em write_file de arquivo existente, delete_file e rename_file"`
	Item       *int        `json:"item,omitempty" desc:"set_text: índice do item repetido (data-vz-i)"`
	Text       *string     `json:"text,omitempty" desc:"set_text: **negrito**, *itálico*, [rótulo](url)"`
	Name       string      `json:"name,omitempty" desc:"atributo, token (--cor) ou knob"`
	Value      *string     `json:"value,omitempty" desc:"valor do atributo, token ou knob"`
	Add        []string    `json:"add,omitempty" desc:"set_classes: classes a pôr"`
	Remove     []string    `json:"remove,omitempty" desc:"set_classes: classes a tirar"`
	Breakpoint string      `json:"breakpoint,omitempty" enum:"sm,md,lg,xl" desc:"set_classes: só a partir dessa largura"`
	MediaID    string      `json:"media_id,omitempty" desc:"replace_image: imagem da biblioteca"`
	Alt        *string     `json:"alt,omitempty" desc:"replace_image: texto alternativo"`
	Before     string      `json:"before,omitempty" desc:"move: node"`
	After      string      `json:"after,omitempty" desc:"move: node"`
	Into       string      `json:"into,omitempty" desc:"move: node que recebe por último"`
	At         *siteAtArgs `json:"at,omitempty" desc:"insert_section: onde"`
	HTML       string      `json:"html,omitempty" desc:"insert_section: o markup da seção"`
	LibraryKey string      `json:"library_key,omitempty" desc:"insert_section: seção pronta da biblioteca"`
	Source     string      `json:"source,omitempty" desc:"insert_section: copia este node (outra página vale)"`
	SourceRev  int         `json:"source_rev,omitempty" desc:"insert_section: rev do arquivo de source"`
	Ref        string      `json:"ref,omitempty" desc:"nome para o que for criado; depois @nome"`
	Variable   string      `json:"variable,omitempty" desc:"define_knob: variável css (--raio)"`
	Kind       string      `json:"kind,omitempty" enum:"range,color" desc:"define_knob"`
	Min        *float64    `json:"min,omitempty" desc:"define_knob range"`
	Max        *float64    `json:"max,omitempty" desc:"define_knob range"`
	Step       *float64    `json:"step,omitempty" desc:"define_knob range"`
	Unit       string      `json:"unit,omitempty" desc:"define_knob range: px, rem, %..."`
	NumberRef  string      `json:"number_ref,omitempty" desc:"set_whatsapp_link: id do número (referências do site_read)"`
}

type siteEditArgs struct {
	Label      string              `json:"label,omitempty" desc:"resumo curto do lote; vira o nome da versão"`
	Operations []siteOperationArgs `json:"operations" req:"true" desc:"aplicadas em ordem, todas ou nenhuma"`
}

type editPayload struct {
	Base    map[string]site.File `json:"base"`
	Steps   []site.StepPatch     `json:"steps"`
	Changes site.FileChanges     `json:"changes"`
	Version int64                `json:"version"`
	Label   string               `json:"label,omitempty"`
}

type editReply struct {
	Files     map[string]uint64 `json:"files"`
	Conflicts []string          `json:"conflicts"`
	Skipped   []int             `json:"skipped"`
}

type editOutcome struct {
	view      *site_usecase.View
	requested int
	changed   []string
	ignored   []string
	conflicts []string
	skipped   []int
	unshown   string
}

type siteEditTool struct {
	siteScope
	deps Deps
}

func NewSiteEditTool(deps Deps) copilot.Tool {
	return &siteEditTool{deps: deps}
}

func (t *siteEditTool) Meta() copilot.Meta { return editMeta() }

func (t *siteEditTool) NeedsApproval(mode copilot.Mode) bool { return mode == copilot.ModeAsk }

func (t *siteEditTool) Definition() tools.Definition {
	return copilottools.Definition("site_edit",
		"Edita o site aberto com um lote de operações aplicadas em ordem, todas ou nenhuma; o editor do usuário mostra cada passo. Campos por op: "+
			"write_file(path, content, rev) arquivo novo, ou inteiro com o rev lido; edit_file(path, old, new, replace_all) troca um trecho exato; delete_file(path, rev); rename_file(path, rev, to); "+
			"set_text(node, rev, text, item); set_attr(node, rev, name, value); set_classes(node, rev, add, remove, breakpoint); replace_image(node, rev, media_id, alt); "+
			"move(node, rev, before ou after); duplicate(node, rev, ref); remove(node, rev); insert_section(at, html ou library_key, ref); "+
			"set_token(name, value) uma variável do tema; set_knob(name, value) um ajuste de knobs.json; define_knob(name, variable, kind, min, max, step, unit); rename_knob(name, to); remove_knob(name); "+
			"set_whatsapp_link(node, rev, number_ref, text) liga um link ao WhatsApp; clear_whatsapp_link(node, rev). "+
			"O retorno traz changed (arquivos com o rev novo), conflicts (onde a versão do usuário ficou), checks e open_gaps; nunca devolve o conteúdo dos arquivos.",
		siteEditArgs{})
}

func (t *siteEditTool) Validate(ctx context.Context, cc copilot.Context, args map[string]interface{}) error {
	if _, _, err := editBatchOf(args); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	if _, err := threadOf(cc); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	if _, err := t.deps.open(ctx, cc, forEditing); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	return nil
}

func (t *siteEditTool) Describe(_ context.Context, _ copilot.Context, args map[string]interface{}) []copilot.Field {
	var a siteEditArgs
	copilottools.BindArgs(args, &a)
	names := make([]string, 0, len(a.Operations))
	for _, op := range a.Operations {
		names = append(names, op.Op)
	}
	fields := []copilot.Field{{Key: "operations", Value: strconv.Itoa(len(names))}, {Key: "steps", Value: strings.Join(names, ", ")}}
	if label := strings.TrimSpace(a.Label); label != "" {
		fields = append(fields, copilot.Field{Key: "label", Value: label})
	}
	return fields
}

func editBatchOf(args map[string]interface{}) (string, json.RawMessage, error) {
	label, _ := args["label"].(string)
	label = strings.TrimSpace(label)
	if len([]rune(label)) > maxEditLabel || strings.ContainsAny(label, "\r\n") {
		return "", nil, errors.New(editLabelBad)
	}
	ops, ok := args["operations"].([]interface{})
	if !ok {
		return "", nil, errors.New(editMalformed)
	}
	if len(ops) == 0 {
		return "", nil, errors.New(editNoOperations)
	}
	raw, err := json.Marshal(ops)
	if err != nil {
		return "", nil, errors.New(editMalformed)
	}
	return label, raw, nil
}

func (t *siteEditTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	label, raw, err := editBatchOf(args)
	if err != nil {
		return failure(err.Error())
	}
	view, err := t.deps.open(ctx, cc, forEditing)
	if err != nil {
		return failure(err.Error())
	}
	outcome, err := t.deps.applyEdit(ctx, cc, view, label, raw)
	if err != nil {
		return failure(err.Error())
	}
	inspectCtx, cancel := bounded(ctx, inspectLimit)
	defer cancel()
	in := t.deps.inspect(inspectCtx, cc, outcome.view)
	return copilot.Result{Status: copilot.StatusOK, Data: withOpenGaps(outcome.data(in), in)}
}

func (o editOutcome) data(in inspection) map[string]interface{} {
	body := map[string]interface{}{"applied": o.requested, "changed": o.changed}
	if len(o.changed) == 0 {
		body["changed"] = []string{}
		body["note"] = editNoChange
	}
	if len(o.ignored) > 0 {
		body["ignored"] = o.ignored
	}
	if len(o.conflicts) > 0 {
		body["conflicts"] = o.conflicts
	}
	if len(o.skipped) > 0 {
		body["skipped_in_editor"] = o.skipped
	}
	if o.unshown != "" {
		body["editor"] = o.unshown
	}
	if len(in.checks) > 0 {
		body["checks"] = in.checks
	}
	return body
}

func operationNames(raw json.RawMessage) []string {
	var ops []struct {
		Op string `json:"op"`
	}
	if json.Unmarshal(raw, &ops) != nil {
		return nil
	}
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.Op
	}
	return names
}

func nameAt(names []string, index int) string {
	if index >= 0 && index < len(names) && names[index] != "" {
		return names[index]
	}
	return "?"
}

func ignoredLines(ignored []site.Ignored) []string {
	lines := make([]string, 0, len(ignored))
	for _, i := range ignored {
		lines = append(lines, fmt.Sprintf(opIgnored, i.Index+1, i.Field))
	}
	return capLines(lines)
}

func refusalMessage(r *site_usecase.EditRefusal, names []string) string {
	var lines []string
	for _, f := range r.Failures {
		words := codeWords[f.Code]
		if words == "" {
			words = opDefault
		}
		code := f.Code
		if f.Field != "" {
			code += " em " + f.Field
		}
		lines = append(lines, fmt.Sprintf(opPrefix, f.Index+1, nameAt(names, f.Index), code+": "+words))
	}
	for _, s := range r.Skipped {
		lines = append(lines, fmt.Sprintf(opSkipped, s.Index+1, s.After+1))
	}
	for _, issue := range r.Issues {
		lines = append(lines, "o lote quebraria uma regra: "+issueLine(issue))
	}
	lines = append(lines, ignoredLines(r.Ignored)...)
	return editRefused + strings.Join(capLines(lines), "; ")
}

func (d Deps) editOnce(ctx context.Context, cc copilot.Context, view *site_usecase.View, author site_usecase.Author, label string, raw json.RawMessage) (*site_usecase.EditResult, error) {
	return d.Storage.Edit(ctx, site_usecase.EditInput{
		WorkspaceID: cc.WorkspaceID, ProjectID: view.Project.ID, ExpectedVersion: view.Project.Version,
		Author: author, Label: label, Batch: raw,
	})
}

func (d Deps) commitEdit(ctx context.Context, cc copilot.Context, view *site_usecase.View, label string, raw json.RawMessage) (*site_usecase.View, *site_usecase.EditResult, error) {
	thread, err := threadOf(cc)
	if err != nil {
		return nil, nil, err
	}
	author := site_usecase.EloAuthor(thread)
	res, err := d.editOnce(ctx, cc, view, author, label, raw)
	if errors.Is(err, shared.ErrVersionConflict) {
		fresh, openErr := d.open(ctx, cc, forEditing)
		if openErr != nil {
			return nil, nil, openErr
		}
		view = fresh
		res, err = d.editOnce(ctx, cc, view, author, label, raw)
	}
	var refused *site_usecase.EditRefusal
	switch {
	case errors.As(err, &refused):
		return nil, nil, errors.New(refusalMessage(refused, operationNames(raw)))
	case errors.Is(err, site.ErrBatchMalformed):
		return nil, nil, errors.New(editMalformed)
	case errors.Is(err, site_usecase.ErrBuilderUnavailable):
		return nil, nil, errors.New(editNoBuilder)
	case errors.Is(err, shared.ErrVersionConflict):
		return nil, nil, errors.New(editMoved)
	case errors.Is(err, site_usecase.ErrProjectArchived):
		return nil, nil, errors.New(siteArchived)
	case errors.Is(err, site_usecase.ErrProjectSuspended):
		return nil, nil, errors.New(siteSuspended)
	case err != nil || res == nil:
		log.Printf("[copilot] site_edit %s could not apply the batch: %v", view.Project.ID, err)
		return nil, nil, errors.New(siteUnavailable)
	}
	return view, res, nil
}

func touchedPaths(steps []site.StepPatch) []string {
	seen := map[string]bool{}
	for _, step := range steps {
		for path := range step.Files {
			seen[path] = true
		}
		for _, path := range step.Deleted {
			seen[path] = true
		}
		for _, r := range step.Renamed {
			seen[r.From], seen[r.To] = true, true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func changedLines(before map[string]site.File, after map[string]site.File, touched []string) []string {
	lines := make([]string, 0, len(touched))
	for _, path := range touched {
		next, ok := after[path]
		_, existed := before[path]
		switch {
		case !ok && existed:
			lines = append(lines, path+" apagado")
		case ok:
			lines = append(lines, path+" r"+strconv.FormatUint(next.Rev, 10))
		}
	}
	return lines
}

func (d Deps) applyEdit(ctx context.Context, cc copilot.Context, view *site_usecase.View, label string, raw json.RawMessage) (editOutcome, error) {
	base, res, err := d.commitEdit(ctx, cc, view, label, raw)
	if err != nil {
		return editOutcome{}, err
	}
	after := res.View
	after.Settings = base.Settings
	touched := touchedPaths(res.Steps)
	outcome := editOutcome{
		view: &after, requested: len(operationNames(raw)),
		changed: changedLines(base.Files, after.Files, touched), ignored: ignoredLines(res.Ignored),
	}
	if len(res.Steps) == 0 || len(touched) == 0 {
		return outcome, nil
	}
	payload := editPayload{Base: map[string]site.File{}, Steps: presenceSteps(res.Steps), Changes: after.Changes, Version: after.Project.Version, Label: label}
	for _, path := range touched {
		if file, ok := base.Files[path]; ok {
			payload.Base[path] = file
		}
	}
	reply, err := screencall.Run(ctx, cc, copilot.ScreenEdit, payload, editorWords)
	if err != nil {
		outcome.unshown = editNotShown + err.Error()
		return outcome, nil
	}
	got, err := decodeEditReply(reply.Data)
	if err != nil {
		outcome.unshown = editNotShown + siteUnreadable
		return outcome, nil
	}
	outcome.conflicts = capLines(got.Conflicts)
	outcome.skipped = skippedOperations(res.Steps, got.Skipped)
	return outcome, nil
}

func presenceSteps(steps []site.StepPatch) []site.StepPatch {
	out := make([]site.StepPatch, len(steps))
	for i, step := range steps {
		out[i] = site.StepPatch{Op: step.Op, Index: step.Index, Focus: step.Focus}
	}
	return out
}

func skippedOperations(steps []site.StepPatch, skipped []int) []int {
	seen := map[int]bool{}
	var ops []int
	for _, step := range skipped {
		if step < 0 || step >= len(steps) || seen[steps[step].Index] {
			continue
		}
		seen[steps[step].Index] = true
		ops = append(ops, steps[step].Index+1)
	}
	return ops
}

func decodeEditReply(data []byte) (editReply, error) {
	var got editReply
	if err := strictjson.Decode(data, &got); err != nil || got.Files == nil {
		return editReply{}, errors.New(siteUnreadable)
	}
	return got, nil
}
