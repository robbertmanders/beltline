package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
)

// Marshal encodes v as compact JSON. Strings keep "<", ">" and "&" as they are,
// and nil slices and maps are written as [] and {}.
func Marshal(v any) ([]byte, error) {
	return encode(v, false)
}

// MarshalStorage encodes v as indented JSON, the form written to the data folder.
func MarshalStorage(v any) ([]byte, error) {
	return encode(v, true)
}

// Unmarshal decodes data into v, as json.Unmarshal does.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Canonical rewrites a JSON document compactly with its object keys sorted, so
// two documents that differ only in key order or spacing compare equal.
func Canonical(raw []byte) ([]byte, error) {
	if raw == nil {
		return []byte("null"), nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("jsonx: data after the top-level value")
	}
	return Marshal(tree)
}

// encode writes v compactly, or indented by two spaces when pretty is set.
func encode(v any, pretty bool) ([]byte, error) {
	e := &encoder{}
	if rv := reflect.ValueOf(v); rv.IsValid() {
		if err := typeEncoder(rv.Type())(e, rv); err != nil {
			return nil, err
		}
	} else {
		e.buf = append(e.buf, "null"...)
	}
	if !pretty {
		return e.buf, nil
	}
	var out bytes.Buffer
	out.Grow(2*len(e.buf) + 64)
	if err := json.Indent(&out, e.buf, "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Members reads data as a JSON object, keeping each member's value as raw JSON.
func Members(data []byte) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	return members, nil
}
