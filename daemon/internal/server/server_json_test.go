package server

import (
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// jsonSample is JSONTests.Sample.
type jsonSample struct {
	When  jsonx.Date      `json:"when"`
	Inner jsonSampleInner `json:"inner"`
	B     bool            `json:"b"`
	A     []string        `json:"a"`
}

// jsonSampleInner is JSONTests.Sample.Inner.
type jsonSampleInner struct {
	Zeta  int    `json:"zeta"`
	Alpha string `json:"alpha"`
}

// jsonSampleValue is JSONTests.sample.
var jsonSampleValue = jsonSample{
	When:  jsonx.Date{Time: time.Unix(1_791_331_200, 0).UTC()},
	Inner: jsonSampleInner{Zeta: 2, Alpha: "x/y"},
	B:     true,
	A:     []string{"second", "first"},
}

// TestKeepsFieldOrderAndWritesISO8601Dates
func TestKeepsFieldOrderAndWritesISO8601Dates(t *testing.T) {
	t.Parallel()
	data, err := routes.JSONEncode(jsonSampleValue)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"when":"2026-10-07T00:00:00Z","inner":{"zeta":2,"alpha":"x/y"},"b":true,"a":["second","first"]}`
	if string(data) != want {
		t.Fatalf("got %s, want %s", data, want)
	}
	var back jsonSample
	if err := routes.JSONDecode(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Inner != jsonSampleValue.Inner || back.B != jsonSampleValue.B || !back.When.Equal(jsonSampleValue.When.Time) {
		t.Fatalf("round trip: %+v", back)
	}
}

// TestEncodesTheSameBytesEveryTime
func TestEncodesTheSameBytesEveryTime(t *testing.T) {
	t.Parallel()
	first, err := routes.JSONEncode(jsonSampleValue)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := routes.JSONEncode(jsonSampleValue)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("encoding %d differs", i)
		}
	}
	dictionary := map[string]int{"b": 1, "a": 2, "c": 3, "aa": 4}
	data, err := routes.JSONEncode(dictionary)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":2,"aa":4,"b":1,"c":3}` {
		t.Fatalf("dictionary: %s", data)
	}
}
