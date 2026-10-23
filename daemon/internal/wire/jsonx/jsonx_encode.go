package jsonx

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// encoder writes one value as compact JSON, as encoding/json does without HTML
// escaping, but with nil slices and maps as [] and {}, apart from the
// exceptions keepNil describes.
type encoder struct {
	buf []byte
	// keepNil is set inside a value that encoding/json would have written
	// untouched: an array, a struct with an unexported embedded field, or a
	// field tagged omitzero or omitempty that is zero. A nil slice or map
	// there is written as null, as the data folder and the wire always did.
	keepNil bool
	depth   int
}

// maxDepth bounds the nesting of pointers, slices and maps, so a cyclic value
// fails with an error instead of exhausting the stack.
const maxDepth = 1000

// reserve makes room for n more bytes. The buffer doubles when it is full, so
// a long output is copied a few times rather than once per growth step, which
// is what append does for large slices.
func (e *encoder) reserve(n int) {
	if cap(e.buf)-len(e.buf) >= n {
		return
	}
	grown := make([]byte, len(e.buf), 2*cap(e.buf)+n)
	copy(grown, e.buf)
	e.buf = grown
}

// encodeFunc writes v to e. A plan is built once per type and reused.
type encodeFunc func(e *encoder, v reflect.Value) error

var (
	marshalerType     = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	numberType        = reflect.TypeFor[json.Number]()
)

var (
	// encoderCache holds the finished plan of each type.
	encoderCache sync.Map
	// encoderLock serializes building: a plan refers to the plans of the
	// types its fields hold, which may be built on the same call.
	encoderLock sync.Mutex
)

// typeEncoder returns the plan for t, building it on first use.
func typeEncoder(t reflect.Type) encodeFunc {
	if f, ok := encoderCache.Load(t); ok {
		return f.(encodeFunc)
	}
	encoderLock.Lock()
	defer encoderLock.Unlock()
	b := &builder{building: map[reflect.Type]*indirect{}, done: map[reflect.Type]encodeFunc{}}
	f := b.get(t)
	// Publish only once every plan of this type tree is complete, so a plan
	// another goroutine loads is never a half-built one.
	for typ, fn := range b.done {
		encoderCache.Store(typ, fn)
	}
	return f
}

// indirect is a plan whose function is set after the plan is referred to,
// for types that contain themselves.
type indirect struct{ fn encodeFunc }

// builder builds the plans of one type tree.
type builder struct {
	building map[reflect.Type]*indirect
	done     map[reflect.Type]encodeFunc
}

// get returns the plan for t, reusing one built earlier in this tree.
func (b *builder) get(t reflect.Type) encodeFunc {
	if f, ok := encoderCache.Load(t); ok {
		return f.(encodeFunc)
	}
	if f, ok := b.done[t]; ok {
		return f
	}
	if p, ok := b.building[t]; ok {
		return func(e *encoder, v reflect.Value) error { return p.fn(e, v) }
	}
	p := &indirect{}
	b.building[t] = p
	p.fn = b.build(t, true)
	b.done[t] = p.fn
	return p.fn
}

// build chooses the plan for t, in encoding/json's order: a MarshalJSON
// method, then a MarshalText method, then the kind of t. A method declared on
// the pointer type only applies to an addressable value (allowAddr).
func (b *builder) build(t reflect.Type, allowAddr bool) encodeFunc {
	if allowAddr && t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) {
		return condAddr(marshalerEncoder(t, true), b.build(t, false))
	}
	if t.Implements(marshalerType) {
		return marshalerEncoder(t, false)
	}
	if allowAddr && t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(textMarshalerType) {
		return condAddr(textEncoder(t, true), b.build(t, false))
	}
	if t.Implements(textMarshalerType) {
		return textEncoder(t, false)
	}
	return b.kindEncoder(t)
}

// addressed returns the address of an addressable non-pointer value, so that
// calling its methods through an interface does not copy the value. The
// method set of the pointer holds every method of the value, so the call is
// the same one.
func addressed(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Pointer && v.CanAddr() {
		return v.Addr()
	}
	return v
}

// condAddr uses ifAddr for an addressable value and ifNot otherwise, as
// encoding/json does for a method declared on the pointer type only.
func condAddr(ifAddr, ifNot encodeFunc) encodeFunc {
	return func(e *encoder, v reflect.Value) error {
		if v.CanAddr() {
			return ifAddr(e, v)
		}
		return ifNot(e, v)
	}
}

// marshalerEncoder calls MarshalJSON and writes its output compacted, with
// U+2028 and U+2029 escaped, as encoding/json does.
// A nil pointer is null. addr calls the method through the value's address.
func marshalerEncoder(t reflect.Type, addr bool) encodeFunc {
	return func(e *encoder, v reflect.Value) error {
		if !addr && v.Kind() == reflect.Pointer && v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		v = addressed(v)
		m, ok := v.Interface().(json.Marshaler)
		if !ok {
			return fmt.Errorf("jsonx: %v does not implement MarshalJSON", t)
		}
		out, err := m.MarshalJSON()
		if err != nil {
			return fmt.Errorf("json: error calling MarshalJSON for type %v: %w", t, err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, out); err != nil {
			return fmt.Errorf("json: error calling MarshalJSON for type %v: %w", t, err)
		}
		out = bytes.ReplaceAll(compact.Bytes(), []byte("\u2028"), []byte(`\u2028`))
		e.buf = append(e.buf, bytes.ReplaceAll(out, []byte("\u2029"), []byte(`\u2029`))...)
		return nil
	}
}

// textEncoder writes the text of MarshalText as a JSON string.
func textEncoder(t reflect.Type, addr bool) encodeFunc {
	return func(e *encoder, v reflect.Value) error {
		if !addr && v.Kind() == reflect.Pointer && v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		v = addressed(v)
		m, ok := v.Interface().(encoding.TextMarshaler)
		if !ok {
			return fmt.Errorf("jsonx: %v does not implement MarshalText", t)
		}
		text, err := m.MarshalText()
		if err != nil {
			return fmt.Errorf("json: error calling MarshalText for type %v: %w", t, err)
		}
		e.buf = appendString(e.buf, string(text))
		return nil
	}
}

// kindEncoder is the plan for t by its kind, ignoring its methods.
func (b *builder) kindEncoder(t reflect.Type) encodeFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder
	case reflect.Float32:
		return floatEncoder(32)
	case reflect.Float64:
		return floatEncoder(64)
	case reflect.String:
		if t == numberType {
			return numberEncoder
		}
		return stringEncoder
	case reflect.Struct:
		return b.structEncoder(t)
	case reflect.Map:
		return b.mapEncoder(t)
	case reflect.Slice:
		return b.sliceEncoder(t)
	case reflect.Array:
		return b.arrayEncoder(t)
	case reflect.Pointer:
		return b.pointerEncoder(t)
	case reflect.Interface:
		return interfaceEncoder
	}
	return unsupportedEncoder(t)
}

// unsupportedEncoder fails for a type JSON cannot represent: a channel, a
// function or a complex number.
func unsupportedEncoder(t reflect.Type) encodeFunc {
	return func(*encoder, reflect.Value) error { return &json.UnsupportedTypeError{Type: t} }
}

// interfaceEncoder writes the value an interface holds, or null when it is nil.
func interfaceEncoder(e *encoder, v reflect.Value) error {
	if v.IsNil() {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	elem := v.Elem()
	return typeEncoder(elem.Type())(e, elem)
}

// pointerEncoder writes the value a pointer points to, or null for nil.
func (b *builder) pointerEncoder(t reflect.Type) encodeFunc {
	elem := b.get(t.Elem())
	return func(e *encoder, v reflect.Value) error {
		if v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		if e.depth >= maxDepth {
			return &json.UnsupportedValueError{Value: v, Str: "encountered a cycle"}
		}
		e.depth++
		err := elem(e, v.Elem())
		e.depth--
		return err
	}
}

// arrayEncoder writes the elements of an array. encoding/json's copy never
// reached an array, so nil collections under one stay null.
