package deps

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestAnUnboundHandleFailsClosed(t *testing.T) {
	h := NewHandle[port]("smart campaigns")
	got, err := h.Get()
	if got != nil {
		t.Fatalf("want the zero value, got %v", got)
	}
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("want ErrUnbound, got %v", err)
	}
	if err.Error() != "dependency not bound yet: smart campaigns" {
		t.Fatalf("message = %q", err.Error())
	}
	if h.Bound() {
		t.Fatal("want unbound")
	}
}

func TestABoundHandleReturnsItsTarget(t *testing.T) {
	h := NewHandle[port]("smart campaigns")
	target := &impl{}
	if err := h.Bind(target); err != nil {
		t.Fatalf("bind: %v", err)
	}
	got, err := h.Get()
	if err != nil || got != port(target) {
		t.Fatalf("got %v, %v", got, err)
	}
	if !h.Bound() || h.Name() != "smart campaigns" {
		t.Fatal("want bound and named")
	}
}

func TestAHandleRefusesNilAndTypedNilTargets(t *testing.T) {
	h := NewHandle[port]("pause all")
	var nilImpl *impl
	if err := h.Bind(nil); !errors.Is(err, ErrNilBinding) {
		t.Fatalf("want ErrNilBinding for nil, got %v", err)
	}
	if err := h.Bind(nilImpl); !errors.Is(err, ErrNilBinding) {
		t.Fatalf("want ErrNilBinding for typed nil, got %v", err)
	}
	if h.Bound() {
		t.Fatal("a refused bind must leave the handle unbound")
	}
}

func TestAHandleBindsOnlyOnce(t *testing.T) {
	h := NewHandle[port]("forms")
	if err := h.Bind(&impl{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if err := h.Bind(&impl{}); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("want ErrAlreadyBound, got %v", err)
	}
}

func TestAHandleIsSafeForConcurrentReadersDuringBind(t *testing.T) {
	h := NewHandle[port]("listener")
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.Get()
		}()
	}
	if err := h.Bind(&impl{}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	wg.Wait()
}

func TestHandlesVerifyNamesEveryUnboundHandle(t *testing.T) {
	var all Handles
	bound := Tracked[port](&all, "bound one")
	Tracked[port](&all, "zeta")
	Tracked[port](&all, "alpha")
	if err := bound.Bind(&impl{}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	err := all.VerifyBound()
	var unbound *UnboundError
	if !errors.As(err, &unbound) {
		t.Fatalf("want an UnboundError, got %v", err)
	}
	if !reflect.DeepEqual(unbound.Names, []string{"alpha", "zeta"}) {
		t.Fatalf("names = %v", unbound.Names)
	}
	if !errors.Is(err, ErrUnbound) {
		t.Fatal("want errors.Is(err, ErrUnbound)")
	}
}

func TestHandlesVerifyPassesWhenEveryHandleIsBound(t *testing.T) {
	var all Handles
	if err := all.VerifyBound(); err != nil {
		t.Fatalf("an empty set is bound: %v", err)
	}
	h := Tracked[port](&all, "one")
	if err := h.Bind(&impl{}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := all.VerifyBound(); err != nil {
		t.Fatalf("want bound, got %v", err)
	}
}
