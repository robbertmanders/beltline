package jsonx

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
)

// The functions in this file are the reference the plans must match byte for
// byte: a copy of the value with nil collections made empty, then encoding/json
// without HTML escaping.

// refMarshal is Marshal as the reference encoder writes it.
func refMarshal(v any) ([]byte, error) { return refEncode(v, "") }

// refMarshalStorage is MarshalStorage as the reference encoder writes it.
func refMarshalStorage(v any) ([]byte, error) { return refEncode(v, "  ") }

// refEncode writes v compactly, or with the given indent.
func refEncode(v any, indent string) ([]byte, error) {
	if rv := reflect.ValueOf(v); rv.IsValid() {
		v = refEmptyCollections(rv).Interface()
	}
	return refWrite(v, indent)
}

// refWrite encodes v with encoding/json, without escaping "<", ">" or "&".
func refWrite(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

var (
	refMarshalerType = reflect.TypeFor[json.Marshaler]()
	refTextType      = reflect.TypeFor[encoding.TextMarshaler]()
)

// refEmptyCollections returns v with every nil slice and map that encodes as a
// JSON array or object made empty. A type with its own marshaler is left alone.
func refEmptyCollections(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	t := v.Type()
	if t.Implements(refMarshalerType) || reflect.PointerTo(t).Implements(refMarshalerType) ||
		t.Implements(refTextType) || reflect.PointerTo(t).Implements(refTextType) {
		return v
	}
	switch v.Kind() {
	case reflect.Struct:
		return refEmptyStruct(v)
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(t, 0, 0)
		}
		out := reflect.MakeSlice(t, v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(refEmptyCollections(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMap(t)
		}
		out := reflect.MakeMapWithSize(t, v.Len())
		for iter := v.MapRange(); iter.Next(); {
			out.SetMapIndex(iter.Key(), refEmptyCollections(iter.Value()))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(t.Elem())
		out.Elem().Set(refEmptyCollections(v.Elem()))
		return out
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(t).Elem()
		out.Set(refEmptyCollections(v.Elem()))
		return out
	}
	return v
}

// refEmptyStruct copies a struct's exported fields with their collections made empty.
func refEmptyStruct(v reflect.Value) reflect.Value {
	t := v.Type()
	for i := range t.NumField() {
		if f := t.Field(i); f.Anonymous && !f.IsExported() {
			return v
		}
	}
	out := reflect.New(t).Elem()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		field := v.Field(i)
		if refOmits(f.Tag.Get("json")) && field.IsZero() {
			out.Field(i).Set(field)
			continue
		}
		out.Field(i).Set(refEmptyCollections(field))
	}
	return out
}

// refOmits reports whether a json tag asks for an omitted zero value.
func refOmits(tag string) bool {
	_, opts, _ := strings.Cut(tag, ",")
	return strings.Contains(","+opts+",", ",omitzero,") || strings.Contains(","+opts+",", ",omitempty,")
}

// referenceCases are values the plans must encode as the reference does.
type referenceEmbedded struct {
	Shared string `json:"shared"`
}

type referenceInner struct {
	Items []string `json:"items"`
	Count int      `json:"count,omitempty"`
}

type referenceOuter struct {
	referenceEmbedded
	*referenceInner
	Zeta     string                `json:"zeta"`
	Alpha    []int                 `json:"alpha"`
	Bytes    []byte                `json:"bytes"`
	NilBytes []byte                `json:"nilBytes"`
	Map      map[string]string     `json:"map"`
	Keys     map[int]bool          `json:"keys"`
	Hidden   []int                 `json:"hidden,omitzero"`
	Empty    []int                 `json:"empty,omitempty"`
	Date     Date                  `json:"date"`
	Dates    []Date                `json:"dates,omitzero"`
	Pointer  *referenceInner       `json:"pointer"`
	Any      any                   `json:"any"`
	Text     referenceText         `json:"text"`
	TextKeys map[referenceText]int `json:"textKeys"`
	Nested   map[string][]float64  `json:"nested"`
	Skipped  string                `json:"-"`
	Private  string                `json:"-,"` //nolint:staticcheck // the "-," tag names the field "-", which is the case under test
	hidden   string
}

// referenceText is a value that encodes as its MarshalText output.
type referenceText struct{ value string }

func (t referenceText) MarshalText() ([]byte, error) { return []byte("<" + t.value + ">"), nil }

// referenceRaw is a value whose MarshalJSON writes unsorted, unescaped JSON.
type referenceRaw struct{}

func (referenceRaw) MarshalJSON() ([]byte, error) {
	return []byte(`{ "b" : [ 1 , 2.50 ], "a" : "<\/ " , "c" : {} }`), nil
}

// referenceUnexported has an unexported embedded struct, so nil collections
// under it stay null in the reference encoder.
type referenceUnexported struct {
	referenceEmbedded
	Items []string `json:"items"`
	Child struct {
		List []int `json:"list"`
	} `json:"child"`
}
