package s3

import (
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

func TestOnlyANotFoundAnswerMeansTheObjectIsMissing(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"not found":    {&smithy.GenericAPIError{Code: "NotFound"}, true},
		"no such key":  {&smithy.GenericAPIError{Code: "NoSuchKey"}, true},
		"forbidden":    {&smithy.GenericAPIError{Code: "Forbidden"}, false},
		"network down": {errors.New("dial tcp: timeout"), false},
	}
	for name, c := range cases {
		if got := missingObject(c.err); got != c.want {
			t.Fatalf("%s: got %v", name, got)
		}
	}
}
