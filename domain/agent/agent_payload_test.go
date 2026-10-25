package agent

import (
	"reflect"
	"strings"
	"testing"
)

func TestAgentPayloadsCarryNoBusinessPhone(t *testing.T) {
	for _, payload := range []interface{}{Agent{}, CreateAgentInput{}, UpdateAgentInput{}, Snapshot{}} {
		kind := reflect.TypeOf(payload)
		for i := 0; i < kind.NumField(); i++ {
			field := kind.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if field.Name == "BusinessPhoneID" || name == "businessPhoneId" {
				t.Fatalf("%s still carries %s; agents reach numbers through the number's receptive settings", kind.Name(), field.Name)
			}
		}
	}
}
