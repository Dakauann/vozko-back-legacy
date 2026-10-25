package container

import (
	"go/ast"
	"go/token"
	"go/types"

	"strings"
	"testing"
	cronPackage "vozko/infra/cron"
)

type containerFunc struct {
	fset *token.FileSet
	decl *ast.FuncDecl
}

func containerFuncs(t *testing.T) map[string]containerFunc {
	t.Helper()
	funcs := map[string]containerFunc{}
	fset := token.NewFileSet()
	parsedFilesWith(t, fset, ".", func(_ string, file *ast.File) {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcs[fn.Name.Name] = containerFunc{fset: fset, decl: fn}
			}
		}
	})
	return funcs
}

func callsOf(node ast.Node, match func(call *ast.CallExpr) bool) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && match(call) {
			found = append(found, call)
		}
		return true
	})
	return found
}

func methodCall(name string, receiver func(string) bool) func(*ast.CallExpr) bool {
	return func(call *ast.CallExpr) bool {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == name && receiver(types.ExprString(sel.X))
	}
}

func failureIsFatal(body ast.Node, start *ast.CallExpr) bool {
	fatal := false
	ast.Inspect(body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok || stmt.Init == nil || stmt.Init.Pos() > start.Pos() || start.End() > stmt.Init.End() {
			return true
		}
		fatal = len(callsOf(stmt.Body, func(call *ast.CallExpr) bool {
			name := types.ExprString(call.Fun)
			return strings.HasPrefix(name, "log.Fatal")
		})) > 0
		return false
	})
	return fatal
}

func callPos(fn containerFunc, receiverAndName string) token.Pos {
	calls := callsOf(fn.decl.Body, func(call *ast.CallExpr) bool {
		return types.ExprString(call.Fun) == receiverAndName
	})
	if len(calls) == 0 {
		return token.NoPos
	}
	return calls[0].Pos()
}

func TestTheUnofficialCampaignConsumerStartsOnceItCanSend(t *testing.T) {
	funcs := containerFuncs(t)
	boot, start := funcs["New"], funcs["startUnofficialWhatsAppCampaigns"]
	if boot.decl == nil || start.decl == nil {
		t.Fatal("New or startUnofficialWhatsAppCampaigns not found")
	}
	if callPos(funcs["initUseCases"], "c.initUnofficialWhatsAppCampaigns") == token.NoPos {
		t.Fatal("initUseCases no longer builds the unofficial campaign bundle")
	}

	started := callPos(boot, "c.startUnofficialWhatsAppCampaigns")
	if started == token.NoPos {
		t.Fatal("New never starts the unofficial campaign consumer; running campaigns would stall after every restart")
	}
	for _, before := range []string{"c.initUseCases", "c.wireUnofficialWhatsAppConversationStack"} {
		at := callPos(boot, before)
		if at == token.NoPos || at > started {
			t.Fatalf("the unofficial campaign consumer starts before %s; it would read a missing bundle or send with no channel adapter", before)
		}
	}

	starts := callsOf(start.decl.Body, methodCall("Start", func(recv string) bool {
		return recv == "c.unofficialWhatsAppCampaigns.Consumer"
	}))
	if len(starts) != 1 || !failureIsFatal(start.decl.Body, starts[0]) {
		t.Fatal("startUnofficialWhatsAppCampaigns must start the consumer once and stop boot when it fails")
	}

	for name, fn := range funcs {
		if name == "startUnofficialWhatsAppCampaigns" {
			continue
		}
		for _, call := range callsOf(fn.decl, methodCall("Start", func(recv string) bool {
			return strings.Contains(recv, "Consumer") && (strings.Contains(recv, "unofficialWhatsAppCampaigns") || strings.HasPrefix(recv, "bundle."))
		})) {
			if name == "initUnofficialWhatsAppCampaigns" || strings.Contains(types.ExprString(call.Fun), "unofficialWhatsAppCampaigns") {
				t.Errorf("%s starts the unofficial campaign consumer at %s; only startUnofficialWhatsAppCampaigns may", name, fn.fset.Position(call.Pos()))
			}
		}
	}
}

func TestEveryAgentSharesTheContainerLoopGuard(t *testing.T) {
	funcs := containerFuncs(t)
	builders := 0
	for name, fn := range funcs {
		for _, call := range callsOf(fn.decl, func(call *ast.CallExpr) bool {
			return types.ExprString(call.Fun) == "loopguard.NewGuard"
		}) {
			builders++
			if name != "loopGuard" {
				t.Errorf("%s builds its own loop guard at %s; use c.loopGuard()", name, fn.fset.Position(call.Pos()))
			}
		}
	}
	if builders != 1 {
		t.Fatalf("the container builds %d loop guards, want exactly one", builders)
	}
}

func aggregateField(expr ast.Expr, aggregates map[string]bool) (string, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	if !ok || !aggregates[inner.Sel.Name] {
		return "", false
	}
	if root, ok := inner.X.(*ast.Ident); !ok || root.Name != "c" {
		return "", false
	}
	return inner.Sel.Name + "." + sel.Sel.Name, true
}

func TestEveryAggregateFieldTheContainerReadsIsAlsoWritten(t *testing.T) {
	aggregates := map[string]bool{"services": true, "useCases": true, "repositories": true}
	literalTypes := map[string]string{"services": "services", "useCases": "useCases", "repositories": "repositories"}
	written := map[string]bool{}
	read := map[string]token.Position{}
	fset := token.NewFileSet()
	parsedFilesWith(t, fset, ".", func(_ string, file *ast.File) {
		lhs := map[ast.Expr]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, target := range node.Lhs {
					lhs[target] = true
					if field, ok := aggregateField(target, aggregates); ok {
						written[field] = true
					}
				}
			case *ast.CompositeLit:
				typeName := strings.TrimPrefix(types.ExprString(node.Type), "*")
				if agg, ok := literalTypes[typeName]; ok {
					for _, elt := range node.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							written[agg+"."+types.ExprString(kv.Key)] = true
						}
					}
				}
			case *ast.SelectorExpr:
				if field, ok := aggregateField(node, aggregates); ok && !lhs[node] {
					if _, seen := read[field]; !seen {
						read[field] = fset.Position(node.Pos())
					}
				}
			}
			return true
		})
	})
	for field, at := range read {
		if !written[field] {
			t.Errorf("c.%s is read at %s but never assigned; whatever reads it always gets nil", field, at)
		}
	}
}


func TestBootVerifiesEveryLateBoundHandle(t *testing.T) {
	boot := containerFuncs(t)["New"]
	verify := callsOf(boot.decl.Body, func(call *ast.CallExpr) bool {
		return types.ExprString(call.Fun) == "c.handles.VerifyBound"
	})
	if len(verify) != 1 || !failureIsFatal(boot.decl.Body, verify[0]) {
		t.Fatal("New must verify every late-bound handle once and stop boot when one is unbound")
	}
	last := boot.decl.Body.List[len(boot.decl.Body.List)-2]
	if last.Pos() > verify[0].Pos() || verify[0].End() > last.End() {
		t.Fatal("the handle check must run last, right before New returns")
	}
}

func TestTheContainerBindsEveryCatalogJobExactlyOnce(t *testing.T) {
	bound := map[string]int{}
	fset := token.NewFileSet()
	parsedFilesWith(t, fset, ".", func(_ string, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || types.ExprString(lit.Type) != "cronJobs" {
				return true
			}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.BasicLit); ok {
						bound[strings.Trim(key.Value, `"`)]++
					}
				}
			}
			return true
		})
	})
	for _, job := range cronPackage.Catalog {
		if bound[job.Name] != 1 {
			t.Errorf("%s is bound %d times, want once", job.Name, bound[job.Name])
		}
		delete(bound, job.Name)
	}
	for name := range bound {
		t.Errorf("%s is bound but not in the cron catalog", name)
	}
}

func TestTheEmailNotificationConsumerStartsAfterEveryEventHandler(t *testing.T) {
	boot := containerFuncs(t)["initUseCases"]
	started := callPos(boot, "c.startEmailSendingConsumer")
	if started == token.NoPos {
		t.Fatal("initUseCases never starts the email provider notification consumer")
	}
	for _, before := range []string{"c.initEmail", "c.initEmailInbox", "c.initEmailAPI"} {
		at := callPos(boot, before)
		if at == token.NoPos || at > started {
			t.Fatalf("the email notification consumer starts before %s; replies, unsubscribes or webhook events in that window would be refused or lost", before)
		}
	}
}

func TestTheConversationHubRunsOnlyAfterItsCommandsAreBound(t *testing.T) {
	start := containerFuncs(t)["startConversationHub"]
	if start.decl == nil {
		t.Fatal("startConversationHub not found")
	}
	binds := callsOf(start.decl.Body, methodCall("BindCommands", func(recv string) bool { return recv == "c.services.conversationHub" }))
	if len(binds) != 1 || !failureIsFatal(start.decl.Body, binds[0]) {
		t.Fatal("startConversationHub must bind the hub commands once and stop boot when the binding fails")
	}
	run := callPos(start, "c.services.conversationHub.Run")
	if run == token.NoPos || run < binds[0].Pos() {
		t.Fatal("the hub must start only after its commands are bound")
	}
}

func TestEveryChannelRegistersItsConversationPortsThroughOneRegistrar(t *testing.T) {
	ports := []string{"SetEntryAccessRepo", "SetConversationStatusStore", "SetConversationCounter", "SetEntryOwnerResolver", "SetContactIdentityLookup", "SetAutomationReader"}
	for name, fn := range containerFuncs(t) {
		if name == "registerConversationChannel" {
			continue
		}
		for _, port := range ports {
			for _, call := range callsOf(fn.decl, methodCall(port, func(string) bool { return true })) {
				t.Errorf("%s registers %s directly at %s; channels go through registerConversationChannel", name, port, fn.fset.Position(call.Pos()))
			}
		}
	}
	boot := containerFuncs(t)["New"]
	sealed := callPos(boot, "c.sealConversationChannels")
	if sealed == token.NoPos {
		t.Fatal("New never seals the channel table")
	}
	for _, stack := range []string{"c.wireInstagramConversationStack", "c.wireFacebookConversationStack", "c.wireTelegramConversationStack", "c.wireMercadoLivreConversationStack", "c.wireTikTokConversationStack", "c.wireWebchatConversationStack", "c.wireEmailInboxConversationStack", "c.wireCarrierConversationStack", "c.wireUnofficialWhatsAppConversationStack"} {
		if at := callPos(boot, stack); at == token.NoPos || at > sealed {
			t.Errorf("%s must run before the channel table is sealed", stack)
		}
	}
}

func assertedSetter(expr ast.Expr) (string, bool) {
	assertion, ok := expr.(*ast.TypeAssertExpr)
	if !ok {
		return "", false
	}
	iface, ok := assertion.Type.(*ast.InterfaceType)
	if !ok || iface.Methods == nil {
		return "", false
	}
	for _, method := range iface.Methods.List {
		for _, name := range method.Names {
			if strings.HasPrefix(name.Name, "Set") {
				return name.Name, true
			}
		}
	}
	return "", false
}

func callsFatal(node ast.Node) bool {
	return node != nil && len(callsOf(node, func(call *ast.CallExpr) bool {
		return strings.HasPrefix(types.ExprString(call.Fun), "log.Fatal")
	})) > 0
}

func TestNoSetterIsReachedThroughASilentTypeAssertion(t *testing.T) {
	for name, fn := range containerFuncs(t) {
		ast.Inspect(fn.decl, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i, stmt := range block.List {
				switch s := stmt.(type) {
				case *ast.IfStmt:
					assign, ok := s.Init.(*ast.AssignStmt)
					if !ok || len(assign.Rhs) != 1 {
						continue
					}
					if setter, found := assertedSetter(assign.Rhs[0]); found && !callsFatal(s.Else) {
						t.Errorf("%s reaches %s through an if-ok assertion with no fatal else at %s", name, setter, fn.fset.Position(s.Pos()))
					}
				case *ast.AssignStmt:
					if len(s.Rhs) != 1 {
						continue
					}
					setter, found := assertedSetter(s.Rhs[0])
					if !found {
						continue
					}
					next, isIf := ast.Stmt(nil), false
					if i+1 < len(block.List) {
						next = block.List[i+1]
						_, isIf = next.(*ast.IfStmt)
					}
					if !isIf || !callsFatal(next.(*ast.IfStmt).Body) {
						t.Errorf("%s reaches %s through an assertion whose failure is not fatal at %s", name, setter, fn.fset.Position(s.Pos()))
					}
				}
			}
			return true
		})
	}
}
