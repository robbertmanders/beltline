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
	return draftTestBelt(SampleStages(TriggerKindManual, SampleAgents(), true))
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
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindManual), boxTestLook)
	if len(m.Nodes) == 0 || m.Nodes[0].ID != "trigger" {
		t.Fatalf("first node is %v, want trigger", m.Nodes)
	}
	if m.Nodes[0].Icon != StageIconHand {
		t.Errorf("trigger icon = %q, want hand", m.Nodes[0].Icon)
	}
	if len(m.Nodes) != len(belt.Stages)+1 {
		t.Errorf("nodes = %d, want %d", len(m.Nodes), len(belt.Stages)+1)
	}
	for index, node := range m.Nodes {
		if node.Row != 0 || node.Column != index {
			t.Errorf("node %s at column %d row %v, want column %d on the line", node.ID, node.Column, node.Row, index)
		}
	}
	if m.Columns != len(m.Nodes) {
		t.Errorf("columns = %d, want %d", m.Columns, len(m.Nodes))
	}
	if len(m.Edges) != len(belt.Stages) {
		t.Errorf("edges = %d, want %d", len(m.Edges), len(belt.Stages))
	}
	for _, edge := range m.Edges {
		if len(edge.Via) != 0 {
			t.Errorf("edge %s>%s turns %v, want straight", edge.From, edge.To, edge.Via)
		}
	}
}

// TestForkRoutesGetRowsOfTheirOwnAndHookedTracks checks that fork routes get rows of their own and hooked tracks.
func TestForkRoutesGetRowsOfTheirOwnAndHookedTracks(t *testing.T) {
	t.Parallel()
	belt := forkedBeltWith([]string{"build"})
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindIssues), boxTestLook)
	fork := belt.Forks[0]
	var pills []BoxMapRoutePill
	for _, pill := range m.Routes {
		if pill.Fork == fork.ID {
			pills = append(pills, pill)
		}
	}
	if len(pills) != len(fork.Routes) {
		t.Errorf("pills = %d, want %d", len(pills), len(fork.Routes))
	}
	rows := map[float64]bool{}
	for _, pill := range pills {
		rows[pill.Row] = true
	}
	if len(rows) != len(pills) {
		t.Errorf("pills share rows: %d rows for %d pills", len(rows), len(pills))
	}
	hooked := false
	// Hooked edges turn twice on one gap, the spine or the gap in front of the box they reach.
	for _, edge := range m.Edges {
		if len(edge.Via) == 0 {
			continue
		}
		hooked = true
		if len(edge.Via) < 2 {
			t.Fatalf("hooked edge %s>%s turns %d times, want at least 2", edge.From, edge.To, len(edge.Via))
		}
		if edge.Via[0].Column != edge.Via[1].Column {
			t.Errorf("hooked edge %s>%s does not turn on one gap", edge.From, edge.To)
		}
	}
	if !hooked {
		t.Error("no hooked edge")
	}
	ids := map[string]bool{}
	for _, node := range m.Nodes {
		ids[node.ID] = true
		if node.Column >= m.Columns {
			t.Errorf("node %s in column %d, past the map's %d", node.ID, node.Column, m.Columns)
		}
	}
	if len(ids) != len(m.Nodes) {
		t.Errorf("node ids are not unique")
	}
}

// TestNestedForksStackTheirRoutesUnderTheParentRoute checks that nested forks stack their routes under the parent route.
func TestNestedForksStackTheirRoutesUnderTheParentRoute(t *testing.T) {
	t.Parallel()
	belt := nfBelt(false, true, true)
	m := BoxLayoutMap(belt.Layout(), false, nil, boxTestLook)
	if len(m.Nodes) != len(belt.Stages) {
		t.Errorf("nodes = %d, want %d", len(m.Nodes), len(belt.Stages))
	}
	forks := map[string]bool{}
	for _, pill := range m.Routes {
		forks[pill.Fork] = true
	}
	if len(forks) != 2 {
		t.Fatalf("forks = %d, want 2", len(forks))
	}
	positions := map[string]bool{}
	for _, node := range m.Nodes {
		if node.Column >= m.Columns {
			t.Errorf("node %s runs past the map", node.ID)
		}
		positions[fmt.Sprintf("%v,%v", node.Column, node.Row)] = true
	}
	if len(positions) != len(m.Nodes) {
		t.Errorf("two nodes share a position")
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
	m := BoxLayoutMap(belt.Layout(), true, new(TriggerKindIssues), boxTestLook)
	if line := boxTestNode(t, m, "trigger").Row; line != 0 {
		t.Fatalf("trigger on row %v, want the line", line)
	}
	var rows []float64
	for _, pill := range m.Routes {
		if pill.Fork == belt.Forks[0].ID {
			rows = append(rows, pill.Row)
		}
	}
	if len(rows) == 0 {
		t.Fatal("no fork")
	}
	low, high := slices.Min(rows), slices.Max(rows)
	if math.Abs((low+high)/2) >= 0.01 {
		t.Errorf("routes are not centred on the line: middle %v", (low+high)/2)
	}
	onLine := slices.ContainsFunc(rows, func(row float64) bool { return math.Abs(row) < 0.01 })
	if onLine && len(rows)%2 == 0 {
		t.Errorf("an even fork puts a route on the line")
	}
}
