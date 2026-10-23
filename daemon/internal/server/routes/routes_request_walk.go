// The request walk: checks a decoded request body against the JSON it came from.
// encoding/json does not reach a type's own UnmarshalJSON with its unknown-field
// rule, and it accepts any string for an enum, so the body is walked here after
// decoding. Data files never go through this walk, so they keep loading unknown
// enum values as they are.

package routes

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// knownCase is a string enum type whose values say whether they are one of its cases.
type knownCase interface{ Known() bool }

// wireShaper is a type whose JSON object is not its struct's own tags: WireShape
// returns the value whose tags name every key the type reads and writes.
type wireShaper interface{ WireShape() any }

// unknownFieldError is a key of a request object that no field of its type takes.
type unknownFieldError string

// Error is `Unknown field "<name>"`.
func (e unknownFieldError) Error() string {
	return `Unknown field "` + string(e) + `"`
}

// notCaseError is a string of a request that is not a case of its enum.
type notCaseError string

func (e notCaseError) Error() string {
	return "not a case of its enum: " + strconv.Quote(string(e))
}

// checkRequestValue checks v, a decoded request value, against node, the JSON it
// was decoded from. It fails on an enum string that is not one of its cases and,
// when keys is set, on an object key that no field of its type takes. The walk
// follows the JSON, so a member the body does not send is never checked, and a
// value a custom decoder has already mapped (a legacy stage type) is checked as it
// now stands.
func checkRequestValue(node any, v reflect.Value, keys bool) error {
	if node == nil || !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return checkRequestValue(node, v.Elem(), keys)
	case reflect.String:
		if _, ok := node.(string); ok {
			return checkRequestCase(v)
		}
	case reflect.Struct:
		members, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		if shaper, ok := v.Interface().(wireShaper); ok {
			v = reflect.ValueOf(shaper.WireShape())
		}
		return checkRequestMembers(members, v, keys)
	case reflect.Slice, reflect.Array:
		items, ok := node.([]any)
		if !ok {
			return nil
		}
		for i, item := range items {
			if i >= v.Len() {
				break
			}
			if err := checkRequestValue(item, v.Index(i), keys); err != nil {
				return err
			}
		}
	case reflect.Map:
		return checkRequestMap(node, v, keys)
	}
	return nil
}

// checkRequestCase fails when v is a string enum that is not one of its cases.
// A string type without Known is not an enum and passes.
func checkRequestCase(v reflect.Value) error {
	if k, ok := v.Interface().(knownCase); ok && !k.Known() {
		return notCaseError(v.String())
	}
	return nil
}

// checkRequestMembers checks an object's members against the struct v: first the
// undeclared keys (when keys is set), then the required fields that are absent or
// null, then each member's value. Names are taken in sorted order, so the same
// body always gets the same message.
func checkRequestMembers(members map[string]any, v reflect.Value, keys bool) error {
	fields := requestFields(v.Type())
	names := slices.Sorted(maps.Keys(members))
	if keys {
		for _, name := range names {
			if _, ok := fields[name]; !ok {
				return unknownFieldError(name)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		if fields[name].required && members[name] == nil {
			return missingFieldError(name)
		}
	}
	for _, name := range names {
		f, ok := fields[name]
		if !ok {
			continue
		}
		field, ok := fieldAt(v, f.index)
		if !ok {
			continue
		}
		if err := checkRequestValue(members[name], field, keys); err != nil {
			return err
		}
	}
	return nil
}

// checkRequestMap checks a map's entries: an enum key must be a case, and each
// value is checked against its member. Keys are visited in sorted order.
func checkRequestMap(node any, v reflect.Value, keys bool) error {
	members, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	type entry struct {
		text  string
		key   reflect.Value
		value reflect.Value
	}
	var entries []entry
	for iter := v.MapRange(); iter.Next(); {
		text, ok := mapKeyText(iter.Key())
		if !ok {
			continue
		}
		entries = append(entries, entry{text: text, key: iter.Key(), value: iter.Value()})
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.text, b.text) })
	for _, e := range entries {
		member, ok := members[e.text]
		if !ok {
			continue
		}
		if e.key.Kind() == reflect.String {
			if err := checkRequestCase(e.key); err != nil {
				return err
			}
		}
		if err := checkRequestValue(member, e.value, keys); err != nil {
			return err
		}
	}
	return nil
}

// mapKeyText is a map key as the JSON member name it was decoded from.
func mapKeyText(k reflect.Value) (string, bool) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), true
	}
	return "", false
}

// requestField is a struct field a request body may name: its index path, its
// type, and whether a body must give it (the tag required:"true").
type requestField struct {
	index    []int
	typ      reflect.Type
	required bool
}

// missingRequiredMember finds a member a body must give that node lacks, at any
// level, from the types alone. It is the check for a body the decoder refused: a
// custom decoder can fail on a value a missing member leaves unread (a stage with
// no type), and the body is then reported as missing that member. A shape that
// depends on a value (a trigger's kind) is read as its zero value; the required
// members of its shapes are the same for every kind.
func missingRequiredMember(node any, t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		members, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		if shaper, ok := reflect.Zero(t).Interface().(wireShaper); ok {
			t = reflect.TypeOf(shaper.WireShape())
		}
		fields := requestFields(t)
		for _, name := range slices.Sorted(maps.Keys(fields)) {
			if fields[name].required && members[name] == nil {
				return missingFieldError(name)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(members)) {
			f, ok := fields[name]
			if !ok || members[name] == nil {
				continue
			}
			if err := missingRequiredMember(members[name], f.typ); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		items, ok := node.([]any)
		if !ok {
			return nil
		}
		for _, item := range items {
			if err := missingRequiredMember(item, t.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		members, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		for _, name := range slices.Sorted(maps.Keys(members)) {
			if err := missingRequiredMember(members[name], t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

// requestFields maps each JSON name a struct takes to its field, with embedded
// structs flattened as encoding/json flattens them.
func requestFields(t reflect.Type) map[string]requestField {
	fields := map[string]requestField{}
	addRequestFields(t, nil, fields)
	return fields
}

// addRequestFields adds the fields of struct type t, reached by the index prefix.
func addRequestFields(t reflect.Type, prefix []int, fields map[string]requestField) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		index := append(slices.Clone(prefix), i)
		if f.Anonymous && name == "" {
			embedded := f.Type
			if embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				addRequestFields(embedded, index, fields)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields[name] = requestField{index: index, typ: f.Type, required: f.Tag.Get("required") == "true"}
	}
}

// fieldAt is the field of struct v at index, through embedded pointers. It fails
// when an embedded pointer on the way is nil.
func fieldAt(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}
