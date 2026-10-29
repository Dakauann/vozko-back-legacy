package main

import (
	"vozko/domain/studio/perception"
	"vozko/usecases/copilot/copilottools/studiotools"
	studio_usecase "vozko/usecases/studio"
)

type blindPerception struct {
	*studio_usecase.PerceptionService
}

func (blindPerception) Look(*perception.Facts) (map[string]any, error) {
	return map[string]any{}, nil
}

func (blindPerception) ArtboardAlerts(facts *perception.Facts) ([][]string, error) {
	return make([][]string, len(facts.Artboards)), nil
}

func (blindPerception) VideoAlerts(*perception.Facts) (map[string]any, error) {
	return map[string]any{}, nil
}

func perceptionFor(arm string) studiotools.StudioPerception {
	if arm == perceptionOff {
		return blindPerception{studio_usecase.NewPerceptionService()}
	}
	return studio_usecase.NewPerceptionService()
}
