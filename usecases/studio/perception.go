package studio_usecase

import (
	"context"
	"log"
	"time"

	"vozko/domain/studio/perception"
)

const slowPerception = 50 * time.Millisecond

type PerceptionService struct {
	now func() time.Time
}

func NewPerceptionService() *PerceptionService {
	return &PerceptionService{now: time.Now}
}

func timed[T any](s *PerceptionService, name string, run func() (T, error)) (T, error) {
	start := s.now()
	out, err := run()
	if took := s.now().Sub(start); took > slowPerception {
		log.Printf("[studio] perception %s took %v", name, took)
	}
	return out, err
}

func (s *PerceptionService) Shapes(ctx context.Context, request *perception.ShapesRequest) ([][]perception.Shape, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return timed(s, "shapes", func() ([][]perception.Shape, error) { return perception.Shapes(request) })
}

func (s *PerceptionService) Checks(facts *perception.Facts) ([]string, error) {
	return timed(s, "checks", func() ([]string, error) { return perception.Checks(facts) })
}

func (s *PerceptionService) Look(facts *perception.Facts) (map[string]any, error) {
	return timed(s, "look", func() (map[string]any, error) { return perception.Look(facts) })
}

func (s *PerceptionService) ArtboardAlerts(facts *perception.Facts) ([][]string, error) {
	return timed(s, "artboard alerts", func() ([][]string, error) { return perception.ArtboardAlerts(facts) })
}

func (s *PerceptionService) VideoAlerts(facts *perception.Facts) (map[string]any, error) {
	return timed(s, "video alerts", func() (map[string]any, error) { return perception.VideoAlerts(facts) })
}
