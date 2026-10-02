// Package wrapper holds small, reusable wire-format wrappers for DTOs —
// currently just Box[T], a discriminated-union type. Lives under pkg/ (not
// internal/) since it has no dependency on this app's domain types and is
// meant to be reusable by any resource's DTO.
package wrapper

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Kind is implemented by every variant of a Box[T]. kind() returns the
// value written to/read from the "type" discriminator field in the JSON.
type Kind interface {
	Kind() string
}

// boxVariant describes one registered (kind string <-> concrete type) pair
// for a given Box[T] instantiation.
type boxVariant struct {
	kind string
	typ  reflect.Type
}

// variantsByBox/fallbackByBox hold the registered variants per T (keyed by
// T's reflect.Type), populated by RegisterVariant/RegisterVariantFallback,
// read by Box[T]'s methods. Go doesn't allow a type parameter on a method,
// so per-T state lives in this package-level side table instead of a field
// on Box[T] itself.
var (
	variantsByBox = map[reflect.Type][]boxVariant{}
	fallbackByBox = map[reflect.Type]boxVariant{}
)

// RegisterVariant wires kind -> V into T's union. Call from a package-level
// var at the file that defines V, once per variant, e.g.:
//
//	var _ = wrapper.RegisterVariant[UserMeta, AdminMeta]("admin")
//
// Panics on a duplicate kind for the same T, or if V has its own "type"
// JSON field — Box.MarshalJSON injects "type" into V's marshaled object,
// silently clobbering a same-named field, so it's rejected at registration
// instead. Both are programming errors, not runtime conditions to recover
// from.
func RegisterVariant[T Kind, V Kind](kind string) bool {
	t := reflect.TypeFor[T]()
	for _, v := range variantsByBox[t] {
		if v.kind == kind {
			panic(fmt.Sprintf("wrapper: duplicate variant kind %q for %s", kind, t))
		}
	}
	vt := reflect.TypeFor[V]()
	if field, ok := hasJSONField(vt, "type"); ok {
		panic(fmt.Sprintf("wrapper: %s.%s uses reserved JSON field \"type\" (Box injects the discriminator there)", vt, field))
	}
	variantsByBox[t] = append(variantsByBox[t], boxVariant{kind, vt})
	return true
}

// hasJSONField reports whether typ (a struct type) has a field that would
// marshal to the given JSON key — by explicit `json:"key"` tag, or by
// field name when untagged (encoding/json's default).
func hasJSONField(typ reflect.Type, key string) (fieldName string, found bool) {
	if typ.Kind() != reflect.Struct {
		return "", false
	}
	for f := range typ.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" {
			name = f.Name
		}
		if name == key {
			return f.Name, true
		}
	}
	return "", false
}

// RegisterVariantFallback registers V as the variant a Box[T] decodes into
// when the JSON's "type" doesn't match any RegisterVariant'd kind, instead
// of UnmarshalJSON erroring — for unions that may see legacy/foreign data
// (e.g. an older row written before a new variant existed) and must not
// break decoding the rest of the row. A Box[T] with no fallback registered
// still hard-errors on an unrecognized kind. At most one fallback per T;
// registering a second panics.
func RegisterVariantFallback[T Kind, V Kind]() bool {
	t := reflect.TypeFor[T]()
	if _, ok := fallbackByBox[t]; ok {
		panic(fmt.Sprintf("wrapper: duplicate fallback variant for %s", t))
	}
	fallbackByBox[t] = boxVariant{typ: reflect.TypeFor[V]()}
	return true
}

// UnknownMeta preserves any payload whose "type" doesn't match a
// registered variant (a legacy row, or one written by a newer app version)
// instead of failing to decode it. Register it as a Box[T]'s fallback:
//
//	var _ = wrapper.RegisterVariantFallback[UserMeta, wrapper.UnknownMeta]()
type UnknownMeta struct {
	Type string
	Raw  json.RawMessage
}

func (m UnknownMeta) Kind() string { return m.Type }

// MarshalJSON returns the original payload verbatim — it already contains
// "type", so Box.MarshalJSON's re-injection of it is a no-op.
func (m UnknownMeta) MarshalJSON() ([]byte, error) { return m.Raw, nil }

// UnmarshalJSON stashes the whole payload verbatim (Box's generic decode
// otherwise field-maps by json tag/name, which would only capture "type"
// and drop everything else this type exists to preserve).
func (m *UnknownMeta) UnmarshalJSON(data []byte) error {
	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &disc); err != nil {
		return err
	}
	m.Type = disc.Type
	m.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// Box wraps a Kind union for JSON (de)serialization and OpenAPI schema
// generation via a "type" discriminator field. Use as a pointer field in a
// DTO; nil means "not set".
type Box[T Kind] struct {
	Value T
}

func (b Box[T]) MarshalJSON() ([]byte, error) {
	if any(b.Value) == nil {
		return []byte("null"), nil
	}
	raw, err := json.Marshal(b.Value)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	kind, err := json.Marshal(b.Value.Kind())
	if err != nil {
		return nil, err
	}
	m["type"] = kind
	return json.Marshal(m)
}

func (b *Box[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		var zero T
		b.Value = zero
		return nil
	}

	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &disc); err != nil {
		return err
	}

	t := reflect.TypeFor[T]()
	for _, v := range variantsByBox[t] {
		if v.kind != disc.Type {
			continue
		}
		return b.decodeInto(data, v.typ)
	}
	if fb, ok := fallbackByBox[t]; ok {
		return b.decodeInto(data, fb.typ)
	}
	return fmt.Errorf("wrapper: unknown %s kind %q", t, disc.Type)
}

func (b *Box[T]) decodeInto(data []byte, typ reflect.Type) error {
	rv := reflect.New(typ)
	if err := json.Unmarshal(data, rv.Interface()); err != nil {
		return err
	}
	b.Value = rv.Elem().Interface().(T)
	return nil
}

// Schema implements huma.SchemaProvider: oneOf + discriminator over T's
// registered variants, instead of huma's "any" fallback for an
// interface-typed field. The fallback variant (if any) is intentionally
// excluded — it's a decode-time safety net, not a shape clients should
// send; unrecognized "type" values still decode fine at runtime, they just
// don't validate against the published schema.
func (Box[T]) Schema(r huma.Registry) *huma.Schema {
	variants := variantsByBox[reflect.TypeFor[T]()]
	oneOf := make([]*huma.Schema, len(variants))
	for i, v := range variants {
		s := r.Schema(v.typ, false, v.typ.Name())
		s.Title = v.typ.Name()
		s.Properties["type"] = &huma.Schema{Type: huma.TypeString, Enum: []any{v.kind}}
		s.Required = append(s.Required, "type")
		oneOf[i] = s
	}
	return &huma.Schema{
		OneOf:         oneOf,
		Discriminator: &huma.Discriminator{PropertyName: "type"},
	}
}
