// The box layout's fork routes and their leads.
package core

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// TestNoTrackRunsAcrossAForkWithoutARouteOnItsRow checks that no track runs across a fork without a route on its row.
func TestNoTrackRunsAcrossAForkWithoutARouteOnItsRow(t *testing.T) {
	t.Parallel()
	belt := boxTestSplitBelt([]boxTestSplitRoute{{ID: "work", Agents: []string{"build"}}, {ID: "skip"}})
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindManual), boxTestLook)
	for _, edge := range m.Edges {
		if edge.From == "triage" && edge.To == "ship" && edge.Route == nil {
			t.Error("a track runs across the fork on the line")
		}
	}
	// The line itself only runs from the trigger to the first stage; everything else is on a route.
	if line := boxTestOnLine(m.Edges); len(line) != 1 || line[0] != "trigger>triage" {
		t.Errorf("line = %v, want [trigger>triage]", line)
	}
}

// TestAnEmptyRouteDrawsItsRowFromTheSpineToWhereTheRoutesMeet checks that an empty route draws its row from the spine to where the routes meet.
func TestAnEmptyRouteDrawsItsRowFromTheSpineToWhereTheRoutesMeet(t *testing.T) {
	t.Parallel()
	belt := boxTestSplitBelt([]boxTestSplitRoute{{ID: "work", Agents: []string{"build"}}, {ID: "skip"}})
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindManual), boxTestLook)
	lane := boxTestEdgeNamed(t, m, "skip")
	if lane.From != "triage" || lane.To != "ship" {
		t.Errorf("empty route runs %s>%s, want triage>ship", lane.From, lane.To)
	}
	if len(lane.Via) == 0 {
		t.Fatal("the empty route's track is straight")
	}
	pill := boxTestRoutePill(t, m, "skip")
	ship := boxTestNode(t, m, "ship")
	if lane.Via[0].Column != pill.Column {
		t.Errorf("track turns first in front of column %d, want the spine %d", lane.Via[0].Column, pill.Column)
	}
	onRow := slices.ContainsFunc(lane.Via, func(turn BoxMapTurn) bool { return math.Abs(turn.Row-pill.Row) < 0.001 })
	if !onRow {
		t.Error("the track does not run along the empty route's row")
	}
	last := lane.Via[len(lane.Via)-1]
	if last.Column != ship.Column || last.Row != ship.Row {
		t.Errorf("track turns last at %v, want in front of the ship on its row (%d, %v)", last, ship.Column, ship.Row)
	}
	// The work route still runs through its own box on the same fork.
	if got := boxTestOnRoute(m.Edges, "work"); fmt.Sprint(got) != fmt.Sprint([]string{"triage>work-build", "work-build>ship"}) {
		t.Errorf("work route edges = %v", got)
	}
}

// TestARouteRunsALeadOfTrackBeforeItsFirstBoxWithItsNameOnIt checks that a route runs a lead of track before its first box with its name on it.
func TestARouteRunsALeadOfTrackBeforeItsFirstBoxWithItsNameOnIt(t *testing.T) {
	t.Parallel()
	belt := boxTestSplitBelt([]boxTestSplitRoute{{ID: "work", Agents: []string{"build"}}, {ID: "skip"}})
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindManual), boxTestLook)
	build := boxTestNode(t, m, "work-build")
	var entry BoxMapEdge
	for _, edge := range m.Edges {
		if edge.Route != nil && *edge.Route == "work" && edge.To == "work-build" {
			entry = edge
		}
	}
	if len(entry.Via) == 0 {
		t.Fatal("the entry into the route does not turn")
	}
	pill := boxTestRoutePill(t, m, "work")
	spine := entry.Via[0].Column
	if spine != pill.Column {
		t.Errorf("name in column %d, want the lead in front of the spine %d", pill.Column, spine)
	}
	if build.Column != spine+1 {
		t.Errorf("first box in column %d, want one lead column past the spine %d", build.Column, spine)
	}
	if entry.Via[len(entry.Via)-1].Row != build.Row || pill.Row != build.Row {
		t.Error("the entry and the name are not on the box's row")
	}
}
