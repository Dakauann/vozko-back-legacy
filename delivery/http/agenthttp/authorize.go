package agenthttp

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/infra/http/middleware"
)

func Authorize(w http.ResponseWriter, r *http.Request, agents agent.GetAgentUseCase) (string, bool) {
	found, ok := AuthorizeAgent(w, r, agents)
	if !ok {
		return "", false
	}
	return found.ID, true
}

func AuthorizeAgent(w http.ResponseWriter, r *http.Request, agents agent.GetAgentUseCase) (*agent.Agent, bool) {
	found, err := agents.Execute(mux.Vars(r)["id"])
	if err != nil {
		WriteError(w, err)
		return nil, false
	}
	if found == nil {
		WriteError(w, agent.ErrAgentNotFound)
		return nil, false
	}
	if found.WorkspaceID != middleware.GetWorkspaceID(r) || !httpx.CanAccessDepartment(r, found.DepartmentID) {
		response.WriteRefusal(w, RefusalForbidden)
		return nil, false
	}
	return found, true
}
