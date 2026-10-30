// Tests of outcomes: watch events save, load and validate, and a Then with
// one target is saved as a string.
// The rest of this file checks the Then and Outcome decoders and encoders.

package core

import (
	"reflect"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestAllWatchEventsSaveLoadAndValidate
func TestAllWatchEventsSaveLoadAndValidate(t *testing.T) {
	t.Parallel()
	signals := 0
	for _, e := range AllWatchEvent {
		if e != WatchEventNone {
			signals++
		}
	}
	if signals != 9 {
		t.Fatalf("signals = %d, want 9", signals)
	}
	if len(AllWatchEvent) != 10 {
		t.Fatalf("WatchEvent cases = %d, want 10", len(AllWatchEvent))
	}
	if WatchEventNone.Label() != "None — none of the other configured outcomes applies" {
		t.Errorf("none label = %q", WatchEventNone.Label())
	}
	if WatchEventNone.WhenClause() != "none of the other configured outcomes applies" {
		t.Errorf("none when clause = %q", WatchEventNone.WhenClause())
	}
	if WatchEventNewChanges != "newChanges" {
		t.Errorf("newChanges raw = %q", WatchEventNewChanges)
	}
	if WatchEventNewChanges.Label() != "the pull request has new changes" {
		t.Errorf("newChanges label = %q", WatchEventNewChanges.Label())
	}
	if _, ok := WatchEventReviewEvents[WatchEventNewChanges]; ok {
		t.Error("newChanges is a review event")
	}
	if got := btDecode[WatchEvent](t, btEncode(t, WatchEventNewChanges)); got != WatchEventNewChanges {
		t.Errorf("round trip = %q", got)
	}
	for _, e := range AllWatchEvent {
		if e.Label() == "" {
			t.Errorf("%q has no label", e)
		}
	}

	// The default belt saves, loads and passes validation with every event on its merge stage.
	belt := SampleBelt([]string{"a/b"}, "agent", SampleAgents(), true)
	var loaded Belt
	if err := json.Unmarshal(vtMarshal(t, belt), &loaded); err != nil {
		t.Fatalf("decode belt: %v", err)
	}
	if got := vtWarnings(loaded); len(got) != 0 {
		t.Errorf("loaded belt warnings %v", got)
	}
	signalSet := map[string]struct{}{}
	for _, e := range AllWatchEvent {
		if e != WatchEventNone {
			signalSet[string(e)] = struct{}{}
		}
	}
	whens := map[string]struct{}{}
	for _, outcome := range loaded.Stages[6].Outcomes {
		whens[outcome.When] = struct{}{}
		if outcome.When == string(WatchEventNone) {
			t.Error("the default merge stage has a none outcome")
		}
	}
	if !reflect.DeepEqual(whens, signalSet) {
		t.Errorf("merge outcomes %v, want %v", whens, signalSet)
	}
}

// TestOneTargetIsSavedAsAString checks how a Then's targets are saved:
// one target is written as a string, several as a list.
func TestOneTargetIsSavedAsAString(t *testing.T) {
	t.Parallel()
	one := btEncode(t, NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"build"}}))
	if !strings.Contains(one, `"to":"build"`) {
		t.Fatalf("one target = %s, want a string", one)
	}
	two := NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"build", "quick-build"}})
	if d := testkit.Diff(two, btDecode[Then](t, btEncode(t, two))); d != "" {
		t.Fatalf("two targets do not round trip:\n%s", d)
	}
}

// TestThenMarshalExactBytes checks the keys Then.encode(to:) writes and the ones it leaves out.
func TestThenMarshalExactBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		then Then
		want string
	}{
		{"plain next", NewThen(ThenArgs{Action: ThenActionNext}), `{"do":"next"}`},
		{"goto with flags", NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"build"}, Max: new(2), WithOutput: true, Comment: true}),
			`{"do":"goto","to":"build","withOutput":true,"max":2,"comment":true}`},
		{"route list", NewThen(ThenArgs{Action: ThenActionRoute, Routes: []string{"a", "b"}}), `{"do":"route","route":["a","b"]}`},
		{"one route", NewThen(ThenArgs{Action: ThenActionRoute, Route: new("complex")}), `{"do":"route","route":"complex"}`},
		{"several targets", NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"x", "y"}}), `{"do":"goto","to":["x","y"]}`},
		{"zero max is kept", NewThen(ThenArgs{Action: ThenActionHold, Max: new(0)}), `{"do":"hold","max":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := btEncode(t, tc.then); got != tc.want {
				t.Fatalf("got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestThenDecodesOneOrListAndDefaults covers Then.init(from:): a scalar or list
// for to and route, and the defaults of the flags.
func TestThenDecodesOneOrListAndDefaults(t *testing.T) {
	t.Parallel()
	bare := btDecode[Then](t, `{"do":"hold"}`)
	if bare.Action != ThenActionHold || len(bare.To) != 0 || len(bare.Routes) != 0 || bare.WithOutput || bare.Max != nil || bare.Comment {
		t.Fatalf("bare hold = %+v, want the defaults", bare)
	}
	nullTo := btDecode[Then](t, `{"do":"hold","to":null,"route":null}`)
	if len(nullTo.To) != 0 || len(nullTo.Routes) != 0 {
		t.Fatalf("null to and route = %+v, want empty", nullTo)
	}
	scalar := btDecode[Then](t, `{"do":"goto","to":"build"}`)
	if d := testkit.Diff([]string{"build"}, scalar.To); d != "" {
		t.Errorf("scalar to:\n%s", d)
	}
	list := btDecode[Then](t, `{"do":"goto","to":["a","b"]}`)
	if d := testkit.Diff([]string{"a", "b"}, list.To); d != "" {
		t.Errorf("list to:\n%s", d)
	}
	route := btDecode[Then](t, `{"do":"route","route":"complex"}`)
	if r := route.Route(); r == nil || *r != "complex" {
		t.Errorf("single route = %v, want complex", r)
	}
	flags := btDecode[Then](t, `{"do":"retry","withOutput":true,"max":3,"comment":true}`)
	if !flags.WithOutput || flags.Max == nil || *flags.Max != 3 || !flags.Comment {
		t.Errorf("flags = %+v", flags)
	}
}

// TestThenSetRouteReplacesTheSelection covers the single-route accessors.
func TestThenSetRouteReplacesTheSelection(t *testing.T) {
	t.Parallel()
	then := NewThen(ThenArgs{Action: ThenActionRoute, Routes: []string{"a", "b"}})
	if r := then.Route(); r == nil || *r != "a" {
		t.Fatalf("route = %v, want a", r)
	}
	then.SetRoute(new("c"))
	if d := testkit.Diff([]string{"c"}, then.Routes); d != "" {
		t.Errorf("after SetRoute(c):\n%s", d)
	}
	then.SetRoute(nil)
	if len(then.Routes) != 0 || then.Route() != nil {
		t.Errorf("after SetRoute(nil), routes = %v", then.Routes)
	}
}

// TestOutcomeMarshalExactBytes checks the synthesized keys of an outcome and
// that its then is written as Then.encode(to:) writes it.
func TestOutcomeMarshalExactBytes(t *testing.T) {
	t.Parallel()
	outcome := Outcome{ID: "o", Name: "merged", When: "merged", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})}
	want := `{"id":"o","name":"merged","when":"merged","tone":"good","then":{"do":"next"}}`
	if got := btEncode(t, outcome); got != want {
		t.Fatalf("outcome JSON\n got %s\nwant %s", got, want)
	}
	if d := testkit.Diff(outcome, btDecode[Outcome](t, want)); d != "" {
		t.Fatalf("outcome round trip:\n%s", d)
	}
}

// TestWatchTargetLabelOf covers WatchTarget.label(_:), which falls back to the raw value.
func TestWatchTargetLabelOf(t *testing.T) {
	t.Parallel()
	if got := WatchTargetPullRequest.LabelOf("merged"); got != "the pull request is merged" {
		t.Errorf("pull request merged = %q", got)
	}
	if got := WatchTargetTicket.LabelOf(string(TicketEventNewComment)); got != "someone commented on the ticket" {
		t.Errorf("ticket comment = %q", got)
	}
	if got := WatchTargetTicket.LabelOf("unknown"); got != "unknown" {
		t.Errorf("unknown when = %q, want itself", got)
	}
}

// TestOutcomeNormaliseTrimsAndLowercases covers the name matching of Outcome.normalize.
func TestOutcomeNormaliseTrimsAndLowercases(t *testing.T) {
	t.Parallel()
	if got := outcomeNormalize("  Checks Failing\n"); got != "checks failing" {
		t.Fatalf("normalize = %q", got)
	}
}

// TestApprovalPrimaryIsTheFirstGoodOutcomeOrTheFirst pins which outcome an
// approval's wide button sits on, and that there is none without outcomes.
func TestApprovalPrimaryIsTheFirstGoodOutcomeOrTheFirst(t *testing.T) {
	t.Parallel()
	if got := ApprovalPrimary(nil); got != nil {
		t.Fatalf("no outcomes: primary = %+v, want nil", got)
	}
	problem := Outcome{ID: "rejected", Tone: ToneProblem}
	good := Outcome{ID: "approved", Tone: ToneGood}
	laterGood := Outcome{ID: "later", Tone: ToneGood}
	if got := ApprovalPrimary([]Outcome{problem, good, laterGood}); got == nil || got.ID != "approved" {
		t.Fatalf("good outcome: primary = %+v, want approved", got)
	}
	if got := ApprovalPrimary([]Outcome{problem, {ID: "other", Tone: ToneNeutral}}); got == nil || got.ID != "rejected" {
		t.Fatalf("no good outcome: primary = %+v, want the first", got)
	}
}

// TestOutcomeButtonLabelIsTheWhenElseTheName pins the words an approval button
// shows: the outcome's when, and its name when there is none.
func TestOutcomeButtonLabelIsTheWhenElseTheName(t *testing.T) {
	t.Parallel()
	if got := (Outcome{Name: "Approved", When: "the plan is right"}).ButtonLabel(); got != "the plan is right" {
		t.Fatalf("label with a when = %q, want the when", got)
	}
	if got := (Outcome{Name: "Approved"}).ButtonLabel(); got != "Approved" {
		t.Fatalf("label without a when = %q, want the name", got)
	}
}

// TestOutcomeDecodesStrictKeys checks an outcome's keys: each one the decoder reads
// with decode is required, and its tone must be a case.
// TestThenDecodesStrictKeys checks that a then names its action, which is required.

// TestDoneAndShipSaveAsTheirOwnActions checks that Then keeps Done and Ship apart: each
// round-trips as its own action, and each has its own label.
func TestDoneAndShipSaveAsTheirOwnActions(t *testing.T) {
	t.Parallel()
	for _, action := range []ThenAction{ThenActionDone, ThenActionShip} {
		then := NewThen(ThenArgs{Action: action})
		data, err := json.Marshal(then)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != `{"do":"`+string(action)+`"}` {
			t.Errorf("%s saved as %s", action, data)
		}
		var back Then
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back.Action != action {
			t.Errorf("%s read back as %s", action, back.Action)
		}
		if !action.MovesOn() || !action.Finishes() {
			t.Errorf("%s: moves on %t finishes %t", action, action.MovesOn(), action.Finishes())
		}
	}
	if ThenActionDone.Label() != "Done" || ThenActionShip.Label() != "Ship" {
		t.Errorf("labels %q and %q, want Done and Ship", ThenActionDone.Label(), ThenActionShip.Label())
	}
	if ThenActionHold.Finishes() || ThenActionNext.Finishes() {
		t.Errorf("hold and next must not finish a run")
	}
}
