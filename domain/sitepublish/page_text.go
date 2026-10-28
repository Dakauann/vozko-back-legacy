package sitepublish

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

const (
	MaxDynamicTextLength = 80
	dynamicTextTag       = "vz-text"
	variantAttr          = "data-vz-variant"
)

func PersonalizePage(body []byte, variant string, param func(name string) string) ([]byte, error) {
	if variant == "" && !bytes.Contains(body, []byte("<"+dynamicTextTag)) {
		return body, nil
	}
	var out bytes.Buffer
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			if errors.Is(z.Err(), io.EOF) {
				return out.Bytes(), nil
			}
			return nil, z.Err()
		case html.StartTagToken:
			token := z.Token()
			switch {
			case token.Data == "html" && variant != "":
				out.WriteString(withAttr(token, variantAttr, variant).String())
			case token.Data == dynamicTextTag:
				out.Write(z.Raw())
				if err := writeDynamicText(&out, z, token, param); err != nil {
					return nil, err
				}
			default:
				out.Write(z.Raw())
			}
		default:
			out.Write(z.Raw())
		}
	}
}

func withAttr(token html.Token, key, value string) html.Token {
	attrs := make([]html.Attribute, 0, len(token.Attr)+1)
	for _, a := range token.Attr {
		if a.Key != key {
			attrs = append(attrs, a)
		}
	}
	token.Attr = append(attrs, html.Attribute{Key: key, Val: value})
	return token
}

func attr(token html.Token, key string) (string, bool) {
	for _, a := range token.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func writeDynamicText(out *bytes.Buffer, z *html.Tokenizer, start html.Token, param func(string) string) error {
	var inner bytes.Buffer
	depth := 1
	for depth > 0 {
		kind := z.Next()
		if kind == html.ErrorToken {
			out.Write(inner.Bytes())
			if errors.Is(z.Err(), io.EOF) {
				return nil
			}
			return z.Err()
		}
		name, _ := z.TagName()
		switch {
		case kind == html.StartTagToken && string(name) == dynamicTextTag:
			depth++
		case kind == html.EndTagToken && string(name) == dynamicTextTag:
			depth--
		}
		if depth > 0 {
			inner.Write(z.Raw())
		}
	}
	name, _ := attr(start, "param")
	fallback, hasFallback := attr(start, "fallback")
	value := ""
	if name != "" {
		value = DynamicText(param(name))
	}
	switch {
	case value != "":
		out.WriteString(html.EscapeString(value))
	case hasFallback:
		out.WriteString(html.EscapeString(fallback))
	default:
		out.Write(inner.Bytes())
	}
	out.Write(z.Raw())
	return nil
}

func DynamicText(raw string) string {
	return cleanText(raw, MaxDynamicTextLength)
}

func cleanText(raw string, maxRunes int) string {
	clean := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw))
	runes := []rune(clean)
	return strings.TrimSpace(string(runes[:min(len(runes), maxRunes)]))
}
