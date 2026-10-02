package wrapper

import (
	"encoding/json"
	"testing"
)

// testShape/testCircle/testSquare are a throwaway second Box[T]
// instantiation, independent of any real DTO's union, proving Box[T] works
// generically.
type testShape interface{ Kind }

type testCircle struct {
	Radius int `json:"radius"`
}

func (testCircle) Kind() string { return "circle" }

type testSquare struct {
	Side int `json:"side"`
}

func (testSquare) Kind() string { return "square" }

var (
	_ = RegisterVariant[testShape, testCircle]("circle")
	_ = RegisterVariant[testShape, testSquare]("square")
)

func TestBox_Generic(t *testing.T) {
	in := Box[testShape]{Value: testCircle{Radius: 5}}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"radius":5,"type":"circle"}` {
		t.Fatalf("unexpected json: %s", raw)
	}

	var out Box[testShape]
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Value != in.Value {
		t.Fatalf("got %#v, want %#v", out.Value, in.Value)
	}
}

func TestBox_Generic_NoFallback_Errors(t *testing.T) {
	var out Box[testShape]
	if err := json.Unmarshal([]byte(`{"type":"triangle"}`), &out); err == nil {
		t.Fatal("expected error: testShape has no fallback registered")
	}
}

type dupShape interface{ Kind }

type dupVariant struct{}

func (dupVariant) Kind() string { return "dup" }

type reservedFieldShape interface{ Kind }

type reservedFieldVariant struct {
	Type string `json:"type"`
}

func (reservedFieldVariant) Kind() string { return "x" }

func TestRegisterVariant_ReservedTypeFieldPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic: variant declares its own \"type\" JSON field")
		}
	}()
	RegisterVariant[reservedFieldShape, reservedFieldVariant]("x")
}

func TestRegisterVariant_DuplicateKindPanics(t *testing.T) {
	RegisterVariant[dupShape, dupVariant]("dup")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate kind")
		}
	}()
	RegisterVariant[dupShape, dupVariant]("dup")
}
