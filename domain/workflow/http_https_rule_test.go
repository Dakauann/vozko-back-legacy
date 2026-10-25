package workflow

import "testing"

func plainHTTPNode(id, url string) Node {
	return Node{ID: id, Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{"url": url, "method": "GET"}}
}

func TestPlainHTTPBlocksANewOrEditedNodeAndOnlyWarnsAnUntouchedPublishedOne(t *testing.T) {
	published := &Graph{Nodes: []Node{plainHTTPNode("n1", "http://legado.test/a"), plainHTTPNode("n2", "http://legado.test/b")}}
	draft := &Graph{Nodes: []Node{
		plainHTTPNode("n1", "http://legado.test/a"),
		plainHTTPNode("n2", "http://legado.test/changed"),
		plainHTTPNode("n3", "HTTP://novo.test"),
		plainHTTPNode("n4", "https://seguro.test"),
		plainHTTPNode("n5", "{{node.n1.url}}"),
	}}
	got := map[string]LintSeverity{}
	for _, issue := range HTTPSIssues(draft, published) {
		if issue.Code != LintHTTPURLNotHTTPS {
			t.Fatalf("code %s", issue.Code)
		}
		got[issue.NodeID] = issue.Severity
	}
	if len(got) != 3 || got["n1"] != SeverityAdvisory || got["n2"] != SeverityBlocking || got["n3"] != SeverityBlocking {
		t.Fatalf("issues %v", got)
	}
	if issues := HTTPSIssues(draft, nil); len(issues) != 3 || issues[0].Severity != SeverityBlocking {
		t.Fatalf("without a published version every plain http node blocks: %+v", issues)
	}
}
