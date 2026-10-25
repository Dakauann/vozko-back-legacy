package node_executors

import (
	"errors"

	"vozko/domain/redact"
	"vozko/domain/workflow"
	"vozko/domain/workspacesecret"
	httpcall_usecase "vozko/usecases/httpcall"
	"vozko/usecases/workflow/scriptvm"
)

type nodeSecrets struct {
	values map[string]string
	scrub  redact.Redactor
}

func resolveNodeSecrets(ctx *workflow.NodeContext, resolver workspacesecret.Resolver, names []string) (nodeSecrets, error) {
	workspaceID := ""
	if ctx.Run != nil {
		workspaceID = ctx.Run.WorkspaceID
	}
	callCtx, release := ctx.CallContext()
	defer release()
	revealed, err := httpcall_usecase.RevealSecrets(callCtx, resolver, workspaceID, names)
	if err != nil {
		return nodeSecrets{}, err
	}
	if len(names) > 0 {
		ctx.Trace.ScrubWith(revealed.Scrub)
	}
	return nodeSecrets{values: revealed.Values, scrub: revealed.Scrub}, nil
}

type scrubbedError struct {
	message string
	cause   error
}

func (e scrubbedError) Error() string { return e.message }

func (e scrubbedError) Unwrap() error { return e.cause }

func (s nodeSecrets) scrubError(err error) error {
	message := s.scrub.String(err.Error())
	if message == err.Error() {
		return err
	}
	return scrubbedError{message: message, cause: err}
}

func requireAllSecrets(names []string, secrets nodeSecrets) error {
	if missing := workspacesecret.MissingNames(names, secrets.values); len(missing) > 0 {
		return &workspacesecret.MissingError{Names: missing}
	}
	return nil
}

func secretFailure(err error) (*workflow.NodeResult, error) {
	if errors.Is(err, workspacesecret.ErrMissing) ||
		errors.Is(err, workspacesecret.ErrUnavailable) ||
		errors.Is(err, workspacesecret.ErrWorkspaceRequired) {
		return &workflow.NodeResult{Error: err.Error(), ErrorKind: workflow.ErrorKindConfig}, nil
	}
	return nil, err
}

type scrubbedState struct {
	state scriptvm.StateAPI
	scrub redact.Redactor
}

func (s scrubbedState) Lookup(path string) (interface{}, bool) { return s.state.Lookup(path) }

func (s scrubbedState) Set(key string, value interface{}) { s.state.Set(key, s.scrub.Value(value)) }
