package jsonx

import (
	"strings"
	"testing"
	"time"
)

// TestDateWritesWholeSecondsInUTC checks the wire form of a date.
func TestDateWritesWholeSecondsInUTC(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 7, 8, 36, 40, 999, time.FixedZone("CEST", 2*3600))
	got, err := Marshal(Date{Time: at})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"2026-10-07T06:36:40Z"` {
		t.Fatalf("date = %s", got)
	}
}

// TestDateReadsOlderForms checks the forms older files hold: fractions and offsets.
func TestDateReadsOlderForms(t *testing.T) {
	t.Parallel()
	for _, text := range []string{`"2026-10-07T06:36:40Z"`, `"2026-10-07T06:36:40.5Z"`, `"2026-10-07T08:36:40+02:00"`} {
		var d Date
		if err := Unmarshal([]byte(text), &d); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if !d.Equal(time.Date(2026, 10, 7, 6, 36, 40, 0, time.UTC)) && !d.Equal(time.Date(2026, 10, 7, 6, 36, 40, 500000000, time.UTC)) {
			t.Fatalf("%s decoded as %v", text, d.Time)
		}
	}
	var d Date
	if err := Unmarshal([]byte(`"yesterday"`), &d); err == nil {
		t.Fatal("a string that is not a date decoded")
	}
}

// TestMarshalWritesEmptyCollections checks what the wire form of a value is:
// fields in declaration order, [] and {} for nil lists and maps, and omitzero kept.
func TestMarshalWritesEmptyCollections(t *testing.T) {
	t.Parallel()
	type inner struct {
		Items []string `json:"items"`
	}
	type value struct {
		Zeta   string            `json:"zeta"`
		Alpha  []int             `json:"alpha"`
		Map    map[string]string `json:"map"`
		Hidden []int             `json:"hidden,omitzero"`
		Inner  *inner            `json:"inner"`
		Text   string            `json:"text"`
	}
	got, err := Marshal(value{Zeta: "<&>", Inner: &inner{}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"zeta":"<&>","alpha":[],"map":{},"inner":{"items":[]},"text":""}`
	if string(got) != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

// TestMarshalStorageWritesTheDataFolderForm checks the indented form of data files.
func TestMarshalStorageWritesTheDataFolderForm(t *testing.T) {
	t.Parallel()
	got, err := MarshalStorage(map[string]any{"b": []int{}, "a": map[string]int{"x": 1}})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n    \"x\": 1\n  },\n  \"b\": []\n}"
	if string(got) != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// TestCanonicalAndMembers checks the two readers of an object's text.
func TestCanonicalAndMembers(t *testing.T) {
	t.Parallel()
	got, err := Canonical([]byte("{ \"b\": 1.50, \"a\": [1, 2] }"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":[1,2],"b":1.50}` {
		t.Fatalf("canonical = %s", got)
	}
	members, err := Members([]byte(`{"a":null,"b":"x"}`))
	if err != nil || len(members) != 2 || string(members["a"]) != "null" {
		t.Fatalf("members = %v, %v", members, err)
	}
	if _, err := Members([]byte(`[1]`)); err == nil || !strings.Contains(err.Error(), "array") {
		t.Fatalf("an array read as members: %v", err)
	}
}
