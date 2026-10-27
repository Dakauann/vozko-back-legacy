package http

import (
	"github.com/gorilla/mux"

	"vozko/delivery/http/agentpublishhttp"
	"vozko/delivery/http/agentrunhttp"
	"vozko/delivery/http/agentscenariohttp"
	"vozko/delivery/http/agenttoolhttp"
	"vozko/delivery/http/automationneedshttp"
)

type AgentRoutes struct {
	Tools      *agenttoolhttp.Handler
	Publishing *agentpublishhttp.Handler
	Runs       *agentrunhttp.Handler
	Scenarios  *agentscenariohttp.Handler
	Needs      *automationneedshttp.Handler
}

func (r *router) setupAgentToolRoutes(protected *mux.Router) {
	if r.agentRoutes.Tools != nil {
		agenttoolhttp.RegisterRoutes(protected, r.agentRoutes.Tools, r.ac)
	}
	if r.agentRoutes.Publishing != nil {
		agentpublishhttp.RegisterRoutes(protected, r.agentRoutes.Publishing, r.ac)
	}
	if r.agentRoutes.Runs != nil {
		agentrunhttp.RegisterRoutes(protected, r.agentRoutes.Runs, r.ac)
	}
	if r.agentRoutes.Scenarios != nil {
		agentscenariohttp.RegisterRoutes(protected, r.agentRoutes.Scenarios, r.ac)
	}
	if r.agentRoutes.Needs != nil {
		automationneedshttp.RegisterRoutes(protected, r.agentRoutes.Needs, r.ac)
	}
}
