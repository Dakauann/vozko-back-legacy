package whatsapplink

import "strings"

const Base = "https://wa.me/"

const hexDigits = "0123456789ABCDEF"

func URL(phoneDigits, text string) string {
	if text == "" {
		return Base + phoneDigits
	}
	return Base + phoneDigits + "?text=" + escapeComponent(text)
}

func escapeComponent(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		b := text[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || strings.IndexByte("-_.!~*'()", b) >= 0 {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexDigits[b>>4])
		out.WriteByte(hexDigits[b&15])
	}
	return out.String()
}
