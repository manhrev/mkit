package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReader(t *testing.T) {
	t.Setenv("PORT", "abc")
	t.Setenv("TTL", "secret-ish")
	t.Setenv("EMPTY", "")
	t.Setenv("LIST", " a, ,b ")
	t.Setenv("ON", "true")
	var r Reader
	if got := r.Key("PORT").Int(5432); got != 5432 {
		t.Fatalf("Int = %d", got)
	}
	if got := r.Key("TTL").Duration(time.Second); got != time.Second {
		t.Fatalf("Duration = %v", got)
	}
	if got := r.Key("EMPTY").String("def"); got != "def" {
		t.Fatalf("String = %q", got)
	}
	if got := r.Key("UNSET").List("*"); !reflect.DeepEqual(got, []string{"*"}) {
		t.Fatalf("List def = %v", got)
	}
	if got := r.Key("LIST").List(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("List = %v", got)
	}
	if !r.Key("ON").Required().Bool(false) {
		t.Fatal("Bool = false")
	}
	r.Key("EMPTY").RequiredIf(false).String("")
	r.Key("UNSET").Required().String("")
	err := r.Err().Error()
	for _, want := range []string{"PORT: invalid value", "TTL: invalid value", "UNSET: required"} {
		if !strings.Contains(err, want) {
			t.Errorf("missing %q in %q", want, err)
		}
	}
	if strings.Contains(err, "abc") || strings.Contains(err, "secret-ish") || strings.Contains(err, "EMPTY") {
		t.Errorf("leak or wrong error: %q", err)
	}
}

func TestAs(t *testing.T) {
	t.Setenv("H", "a=1")
	var r Reader
	parse := func(s string) (map[string]string, error) {
		k, v, _ := strings.Cut(s, "=")
		return map[string]string{k: v}, nil
	}
	if got := r.Key("H").As(map[string]string{}, parse); got["a"] != "1" {
		t.Fatalf("As = %v", got)
	}
}
