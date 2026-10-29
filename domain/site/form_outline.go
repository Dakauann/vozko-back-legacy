package site

import (
	"vozko/domain/siteform"
	"vozko/domain/sitehtml"
)

type OutlineField struct {
	Node      string        `json:"node"`
	Row       string        `json:"row,omitempty"`
	LabelNode string        `json:"labelNode,omitempty"`
	Name      string        `json:"name"`
	Kind      siteform.Kind `json:"kind"`
	Required  bool          `json:"required,omitempty"`
	Label     string        `json:"label,omitempty"`
}

type OutlineForm struct {
	ID     string         `json:"id"`
	Node   string         `json:"node"`
	Submit string         `json:"submit,omitempty"`
	Fields []OutlineField `json:"fields"`
}

func FormOutline(doc *sitehtml.Document) []OutlineForm {
	out := []OutlineForm{}
	node := func(i int) string {
		if i < 0 {
			return ""
		}
		return doc.Elements[i].Node
	}
	for i, el := range doc.Elements {
		id := siteform.FormID(doc, i)
		if id == "" || el.Node == "" {
			continue
		}
		form := OutlineForm{ID: id, Node: el.Node, Fields: []OutlineField{}}
		for _, child := range doc.Descendants(i, "input", "select", "textarea", "button") {
			if siteform.OwningForm(doc, child) != i || doc.Elements[child].Node == "" {
				continue
			}
			control, ok := siteform.ReadControl(doc, child)
			if !ok {
				if form.Submit == "" && submits(doc, child) {
					form.Submit = doc.Elements[child].Node
				}
				continue
			}
			form.Fields = append(form.Fields, OutlineField{
				Node: doc.Elements[child].Node, Row: node(control.RowAt), LabelNode: node(control.LabelTextAt),
				Name: control.Name, Kind: control.Kind, Required: control.Required, Label: clip(control.Label),
			})
		}
		out = append(out, form)
	}
	return out
}

func submits(doc *sitehtml.Document, i int) bool {
	kind, _ := doc.AttrValue(i, "type")
	if doc.Elements[i].Tag == "button" {
		return kind == "" || kind == "submit"
	}
	return kind == "submit" || kind == "image"
}
