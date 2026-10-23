package jsonx

import (
	"strings"
	"testing"
)

// TestMarshalerOutputIsCompacted checks that a MarshalJSON method's output is
// written as it is, compacted, both in the wire form and the storage form.
func TestMarshalerOutputIsCompacted(t *testing.T) {
	t.Parallel()
	value := struct {
		R referenceRaw `json:"r"`
	}{}
	got, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"r\":{\"b\":[1,2.50],\"a\":\"<\\/\\u2028\",\"c\":{}}}"; string(got) != want {
		t.Fatalf("marshaler output = %s, want %s", got, want)
	}
	storage, err := MarshalStorage(value)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"r\": {\n    \"b\": [\n      1,\n      2.50\n    ],\n    \"a\": \"<\\/\\u2028\",\n    \"c\": {}\n  }\n}"
	if string(storage) != want {
		t.Fatalf("storage form = %q, want %q", storage, want)
	}
	if strings.Contains(string(got), " ") {
		t.Fatalf("compact output has whitespace: %s", got)
	}
}
