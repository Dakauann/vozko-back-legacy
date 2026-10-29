package studio_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/studio/perception"
)

const perceptionSurface = `{"canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[{"id":"a","type":"shape","shape":"rect","fill":"#111111","transform":{"x":0.5,"y":0.5,"w":0.3,"h":0.3,"rotation":0,"opacity":1},"effects":[{"id":"d","type":"drop-shadow","color":"#000000","opacity":1,"x":0,"y":30,"blur":0,"spread":0}]}],"texts":{}}`

func TestPerceptionShapesDrawTheFindingsOfEachScene(t *testing.T) {
	request, err := perception.DecodeShapesRequest([]byte(`{"approximate":false,"layers":{"depth":true,"layout":false},"surfaces":[` + perceptionSurface + `,` + perceptionSurface + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	shapes, err := NewPerceptionService().Shapes(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 2 || len(shapes[0]) == 0 {
		t.Fatalf("shapes %v", shapes)
	}
}

func TestPerceptionShapesStopWhenTheRequestIsGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewPerceptionService().Shapes(ctx, &perception.ShapesRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestPerceptionChecksFailClosedWithoutFacts(t *testing.T) {
	if _, err := NewPerceptionService().Checks(&perception.Facts{}); !errors.Is(err, perception.ErrIncomplete) {
		t.Fatalf("err %v", err)
	}
}
