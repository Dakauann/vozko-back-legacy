package workflow

import (
	"fmt"
	"strings"
)

const LintHTTPURLNotHTTPS LintIssueCode = "NODE_HTTP_URL_NOT_HTTPS"

func HTTPSIssues(draft, published *Graph) []LintIssue {
	if draft == nil {
		return nil
	}
	running := map[string]string{}
	if published != nil {
		for _, n := range published.Nodes {
			if n.Type == NodeTypeActionHTTPRequest {
				running[n.ID] = httpNodeURL(n)
			}
		}
	}
	var issues []LintIssue
	for _, n := range draft.Nodes {
		if n.Type != NodeTypeActionHTTPRequest {
			continue
		}
		url := httpNodeURL(n)
		if !strings.HasPrefix(strings.ToLower(url), "http://") {
			continue
		}
		severity := SeverityBlocking
		if previous, ok := running[n.ID]; ok && previous == url {
			severity = SeverityAdvisory
		}
		issues = append(issues, LintIssue{
			Code: LintHTTPURLNotHTTPS, Severity: severity, NodeID: n.ID, Field: "url",
			Message: fmt.Sprintf("O nó %q chama uma URL http, sem criptografia", n.ID),
			Hint:    "Troque para https. Um nó http já publicado continua rodando até ser editado.",
		})
	}
	return issues
}

func httpNodeURL(n Node) string {
	url, _ := n.Config["url"].(string)
	return strings.TrimSpace(url)
}
