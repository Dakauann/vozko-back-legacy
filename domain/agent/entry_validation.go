package agent

import "strings"

func MetadataValueMissing(metadata map[string]any, key string) bool {
	value, exists := metadata[key]
	if !exists {
		return true
	}
	text, isText := value.(string)
	return isText && strings.TrimSpace(text) == ""
}
