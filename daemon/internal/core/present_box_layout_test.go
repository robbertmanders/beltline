//

package core

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// boxTestLook is the look of a stage with no agent.
func boxTestLook(stage Stage) StageLook {
	return StageLookOf(stage, nil)
}

// boxTestStraightBelt is the manual belt's default stages, in one row.
func boxTestStraightBelt() Belt {
	return draftTestBelt(DefaultsStages(TriggerKindManual, DefaultsBuiltInAgents(), true))
}

// boxTestNode is the node with id, failing the test when there is none.
func boxTestNode(t *testing.T, m BoxMap, id string) BoxMapNode {
	t.Helper()
	node := m.Node(id)
	if node == nil {
		t.Fatalf("no node %q", id)
	}
	return *node
}

// boxTestRoutePill is the first route pill of route, failing the test when there is none.
func boxTestRoutePill(t *testing.T, m BoxMap, route string) BoxMapRoutePill {
	t.Helper()
	for _, pill := range m.Routes {
		if pill.Route == route {
			return pill
		}
	}
	t.Fatalf("no route pill for %q", route)
	return BoxMapRoutePill{}
}

// boxTestEdgeNamed is the first edge whose route is route, failing the test when there is none.
func boxTestEdgeNamed(t *testing.T, m BoxMap, route string) BoxMapEdge {
	t.Helper()
	for _, edge := range m.Edges {
		if edge.Route != nil && *edge.Route == route {
			return edge
		}
	}
	t.Fatalf("no edge on route %q", route)
	return BoxMapEdge{}
}

// boxTestFirstSubpath is the first subpath's points of a path, failing the test when there is none.
func boxTestFirstSubpath(t *testing.T, path MapPath) []MapPoint {
	t.Helper()
	if len(path.Subpaths) == 0 {
		t.Fatal("path has no subpaths")
	}
	return path.Subpaths[0]
}

// boxTestEdgeKeys is each edge as "from>to", in order.
func boxTestEdgeKeys(edges []BoxMapEdge) []string {
	var keys []string
	for _, edge := range edges {
		keys = append(keys, edge.From+">"+edge.To)
	}
	return keys
}

// boxTestOnLine is the edges with no route, as "from>to".
func boxTestOnLine(edges []BoxMapEdge) []string {
	var onLine []BoxMapEdge
	for _, edge := range edges {
		if edge.Route == nil {
			onLine = append(onLine, edge)
		}
	}
	return boxTestEdgeKeys(onLine)
}

// boxTestOnRoute is the edges on route, as "from>to".
func boxTestOnRoute(edges []BoxMapEdge, route string) []string {
	var on []BoxMapEdge
	for _, edge := range edges {
		if edge.Route != nil && *edge.Route == route {
			on = append(on, edge)
		}
	}
	return boxTestEdgeKeys(on)
}

// boxTestSplitRoute is one route of splitBelt: its id and the agents its stages run.
type boxTestSplitRoute struct {
	ID     string
	Agents []string
}

// boxTestSplitBelt is triage, then a fork whose routes hold the given agents (none for an empty route), then Ship.
// Each route's stages are named <route>-<agent>.
func boxTestSplitBelt(routes []boxTestSplitRoute) Belt {
	fork := Fork{ID: "split"}
	for _, route := range routes {
		fork.Routes = append(fork.Routes, Route{ID: route.ID, Name: boxTestUpper(route.ID)})
	}
	var stages []Stage
	add := func(agent string, route *string, id string) {
		stage := draftTestStage(agent, StageTypeAgent, agent, "")
		if route != nil {
			stage.Route = route
		}
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	add("triage", nil, "")
	for _, route := range routes {
		for _, agent := range route.Agents {
			r := route.ID
			add(agent, &r, route.ID+"-"+agent)
		}
	}
	add("ship", nil, "")
	return draftTestBelt(stages, fork)
}

// boxTestUpper is the upper-case form of an ASCII route id, as Swift's uppercased() gives it.
func boxTestUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 'a' + 'A'
		}
	}
	return string(out)
}

// TestStraightBeltPutsTheTriggerAndEveryStageOnOneRow checks that straight belt puts the trigger and every stage on one row.
func TestStraightBeltPutsTheTriggerAndEveryStageOnOneRow(t *testing.T) {
	t.Parallel()
	belt := boxTestStraightBelt()
	m := BoxLayoutMap(belt.Layout(), BoxMapMetricsRun, true, new(TriggerKindManual), boxTestLook)
	if len(m.Nodes) == 0 || m.Nodes[0].ID != "trigger" {
		t.Fatalf("first node is %v, want trigger", m.Nodes)
	}
	if m.Nodes[0].Icon != StageIconHand {
		t.Errorf("trigger icon = %q, want hand", m.Nodes[0].Icon)
	}
	if len(m.Nodes) != len(belt.Stages)+1 {
		t.Errorf("nodes = %d, want %d", len(m.Nodes), len(belt.Stages)+1)
	}
	rows := map[float64]bool{}
	for _, node := range m.Nodes {
		rows[node.Y] = true
	}
	if len(rows) != 1 {
		t.Errorf("nodes sit on %d rows, want 1", len(rows))
	}
	if len(m.Edges) != len(belt.Stages) {
		t.Errorf("edges = %d, want %d", len(m.Edges), len(belt.Stages))
	}
	for _, edge := range m.Edges {
		if !edge.Straight {
			t.Errorf("edge %s>%s is not straight", edge.From, edge.To)
		}
	}
	// A + on every join and one past the last box.
	if len(m.Inserts) != len(belt.Stages)+1 {
		t.Errorf("inserts = %d, want %d", len(m.Inserts), len(belt.Stages)+1)
	}
	if last := m.Inserts[len(m.Inserts)-1]; last.Before != nil {
		t.Errorf("last insert has a box after it")
	}
	if len(m.Forks) != 0 {
		t.Errorf("forks = %d, want none", len(m.Forks))
	}
	right := m.Nodes[0].X + m.Nodes[0].Width
	for _, node := range m.Nodes {
		right = max(right, node.X+node.Width)
	}
	if m.Width < right {
		t.Errorf("width %v is narrower than the boxes (%v)", m.Width, right)
	}
}

// TestForkRoutesGetRowsOfTheirOwnAndHookedTracks checks that fork routes get rows of their own and hooked tracks.
func TestForkRoutesGetRowsOfTheirOwnAndHookedTracks(t *testing.T) {
	t.Parallel()
	belt := forkedBeltWith([]string{"build"})
	m := BoxLayoutMap(belt.Layout(), BoxMapMetricsEditor, true, new(TriggerKindIssues), boxTestLook)
	if len(m.Forks) != 1 {
		t.Fatalf("forks = %d, want 1", len(m.Forks))
	}
	fork := m.Forks[0]
	var pills []BoxMapRoutePill
	for _, pill := range m.Routes {
		if pill.Fork == fork.ID {
			pills = append(pills, pill)
		}
	}
	if len(pills) != fork.Routes {
		t.Errorf("pills = %d, want %d", len(pills), fork.Routes)
	}
	ys := map[float64]bool{}
	for _, pill := range pills {
		ys[pill.Y] = true
	}
	if len(ys) != len(pills) {
		t.Errorf("pills share rows: %d rows for %d pills", len(ys), len(pills))
	}
	hooked := slices.ContainsFunc(m.Edges, func(edge BoxMapEdge) bool {
		return !edge.Straight && edge.Path.CornerRadius == BoxMapMetricsEditor.Corner
	})
	if !hooked {
		t.Error("no hooked edge with the editor's corner radius")
	}
	// Hooked edges turn twice, on the spine or the gap before the box they reach (an empty route's track turns four times).
	for _, edge := range m.Edges {
		if edge.Straight {
			continue
		}
		points := boxTestFirstSubpath(t, edge.Path)
		if len(points) < 4 {
			t.Fatalf("hooked edge %s>%s has %d points, want at least 4", edge.From, edge.To, len(points))
		}
		if points[1].X != points[2].X {
			t.Errorf("hooked edge %s>%s does not turn on its spine", edge.From, edge.To)
		}
	}
	ids := map[string]bool{}
	for _, node := range m.Nodes {
		ids[node.ID] = true
	}
	if len(ids) != len(m.Nodes) {
		t.Errorf("node ids are not unique")
	}
	for _, node := range m.Nodes {
		if node.Y+node.Height > m.Height {
			t.Errorf("node %s runs past the map's height", node.ID)
		}
	}
}

// TestNestedForksStackTheirRoutesUnderTheParentRoute checks that nested forks stack their routes under the parent route.
func TestNestedForksStackTheirRoutesUnderTheParentRoute(t *testing.T) {
	t.Parallel()
	belt := nfBelt(false, true, true)
	m := BoxLayoutMap(belt.Layout(), BoxMapMetricsRun, false, nil, boxTestLook)
	if len(m.Forks) != 2 {
		t.Fatalf("forks = %d, want 2", len(m.Forks))
	}
	if len(m.Nodes) != len(belt.Stages) {
		t.Errorf("nodes = %d, want %d", len(m.Nodes), len(belt.Stages))
	}
	positions := map[string]bool{}
	for _, node := range m.Nodes {
		if node.Y+node.Height > m.Height || node.X+node.Width > m.Width {
			t.Errorf("node %s runs past the map", node.ID)
		}
		positions[fmt.Sprintf("%v,%v", node.X, node.Y)] = true
	}
	if len(positions) != len(m.Nodes) {
		t.Errorf("two nodes share a position")
	}
}

// TestEveryPlaceAStageCanGoHasAPlusThatAddsThereAtTheRightIndex checks that every place a stage can go has a plus that adds there at the right index.
func TestEveryPlaceAStageCanGoHasAPlusThatAddsThereAtTheRightIndex(t *testing.T) {
	t.Parallel()
	belt := forkedBeltWith([]string{"build"})
	layout := belt.Layout()
	m := BoxLayoutMap(layout, BoxMapMetricsEditor, true, new(TriggerKindIssues), boxTestLook)
	gap := BoxMapMetricsEditor.Gap
	// A + in front of every box (where a route's end is drawn on the same spot, that one serves), and one at the
	// end of every route (or in an empty route's slot).
	for _, node := range m.Nodes {
		if node.Kind == BoxMapKindTrigger {
			continue
		}
		spot := slices.ContainsFunc(m.Inserts, func(insert BoxMapInsert) bool {
			return math.Abs(insert.X-(node.X-gap/2)) < 1 && math.Abs(insert.Y-(node.Y+node.Height/2)) < 1
		})
		if !spot {
			t.Errorf("no + in front of %s", node.ID)
		}
	}
	// A route's end is drawn unless the box after its fork takes the spot (see addInsert).
	for _, fork := range belt.Forks {
		for _, route := range fork.Routes {
			end, taken := false, false
			for _, insert := range m.Inserts {
				if insert.Route != nil && *insert.Route == route.ID && insert.Before == nil {
					end = true
				}
				if insert.Before != nil && insert.Route == nil {
					taken = true
				}
			}
			if !end && !taken {
				t.Errorf("no + at the end of %s", route.ID)
			}
		}
	}
	// Each adds at the index the stage list takes: in front of a box, its own index; at a route's end, the route's end.
	for _, insert := range m.Inserts {
		if insert.Before != nil {
			for index, stage := range belt.Stages {
				if stage.ID == *insert.Before && insert.AddAt != index {
					t.Errorf("insert before %s adds at %d, want %d", stage.ID, insert.AddAt, index)
				}
			}
		}
		if insert.Route != nil && insert.Before == nil {
			if want := layout.End(*insert.Route); insert.AddAt != want {
				t.Errorf("insert at the end of %s adds at %d, want %d", *insert.Route, insert.AddAt, want)
			}
		}
	}
	last := m.Inserts[len(m.Inserts)-1]
	if last.Before != nil || last.Route != nil {
		t.Errorf("last insert is not past the line")
	}
	if last.AddAt != len(belt.Stages) {
		t.Errorf("last insert adds at %d, want %d", last.AddAt, len(belt.Stages))
	}
	spots := map[string]bool{}
	for _, insert := range m.Inserts {
		spots[fmt.Sprintf("%v,%v", insert.X, insert.Y)] = true
	}
	if len(spots) != len(m.Inserts) {
		t.Errorf("two inserts share a spot")
	}
}

// TestAStraightBeltAddsAtEveryIndexInTurn checks that a straight belt adds at every index in turn.
func TestAStraightBeltAddsAtEveryIndexInTurn(t *testing.T) {
	t.Parallel()
	belt := boxTestStraightBelt()
	m := BoxLayoutMap(belt.Layout(), BoxMapMetricsEditor, true, new(TriggerKindManual), boxTestLook)
	if len(m.Inserts) != len(belt.Stages)+1 {
		t.Fatalf("inserts = %d, want %d", len(m.Inserts), len(belt.Stages)+1)
	}
	for index, insert := range m.Inserts {
		if insert.AddAt != index {
			t.Errorf("insert %d adds at %d", index, insert.AddAt)
		}
		if insert.Route != nil {
			t.Errorf("insert %d is on a route", index)
		}
	}
}

// TestAgentIconsFallBackToTheChip checks that an agent without an icon shows the chip, and that its icon
// is written to and read from its file.
func TestAgentIconsFallBackToTheChip(t *testing.T) {
	t.Parallel()
	blue := TintBlue
	agent := NewAgent(AgentArgs{ID: "a", Name: "A", Tint: &blue})
	if agent.Glyph() != StageIconChip {
		t.Fatalf("glyph = %q, want chip", agent.Glyph())
	}
	flag := StageIconFlag
	agent.Icon = &flag
	if agent.Glyph() != StageIconFlag {
		t.Fatalf("glyph with icon = %q, want flag", agent.Glyph())
	}
	file := AgentFileWrite(agent)
	if !nfContains([]string{file}, "icon: flag") {
		t.Errorf("agent file has no icon line:\n%s", file)
	}
	parsed, err := AgentFileParse(file, "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Icon == nil || *parsed.Icon != StageIconFlag {
		t.Errorf("parsed icon = %v, want flag", parsed.Icon)
	}
}

// TestAForkSplitsTheLineEvenlyAboveAndBelowIt checks that a fork splits the line evenly above and below it.
func TestAForkSplitsTheLineEvenlyAboveAndBelowIt(t *testing.T) {
	t.Parallel()
	belt := forkedBeltWith([]string{"build"})
	m := BoxLayoutMap(belt.Layout(), BoxMapMetricsRun, true, new(TriggerKindIssues), boxTestLook)
	line := boxTestNode(t, m, "trigger").Y
	if len(m.Forks) == 0 {
		t.Fatal("no fork")
	}
	fork := m.Forks[0]
	var ys []float64
	for _, pill := range m.Routes {
		if pill.Fork == fork.ID {
			ys = append(ys, pill.Y)
		}
	}
	centre := line + BoxMapMetricsRun.NodeHeight/2
	low, high := ys[0], ys[0]
	for _, y := range ys {
		low = min(low, y)
		high = max(high, y)
	}
	if math.Abs((low+high)/2-centre) >= 0.01 {
		t.Errorf("routes are not centred on the line: middle %v, line %v", (low+high)/2, centre)
	}
	onLine := slices.ContainsFunc(ys, func(y float64) bool { return math.Abs(y-centre) < 0.01 })
	if onLine && len(ys)%2 == 0 {
		t.Errorf("an even fork puts a route on the line")
	}
	top := m.Nodes[0].Y
	for _, node := range m.Nodes {
		top = min(top, node.Y)
	}
	if top != BoxMapMetricsRun.PadTop {
		t.Errorf("highest node at %v, want %v", top, BoxMapMetricsRun.PadTop)
	}
}
