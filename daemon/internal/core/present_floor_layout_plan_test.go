// The Floor's placement arithmetic: spreading a layer and laying the return lanes.
package core

import (
	"fmt"
	"math"
	"testing"
)

// TestSpreadKeepsOrderAndCentresOverlaps checks that spread keeps order and centres overlaps.
func TestSpreadKeepsOrderAndCentresOverlaps(t *testing.T) {
	t.Parallel()
	metrics := OverviewBoxesMetrics()
	metrics.Gap = 10
	plan := newFloorLayoutPlan(FactoryTrack{}, metrics, func(FactoryTrackStage) float64 { return 0 })
	if got := plan.spread([]float64{0, 100}, []float64{20, 20}); fmt.Sprint(got) != "[0 100]" {
		t.Errorf("spread = %v, want [0 100]", got)
	}
	if got := plan.spread([]float64{50, 50}, []float64{20, 20}); fmt.Sprint(got) != "[35 65]" {
		t.Errorf("spread = %v, want [35 65]", got)
	}
}

// TestAReturnRunsUnderTheBoxesItPassesAndIntoTheEarlierOne checks that a return runs under the boxes it passes and into the earlier one.
func TestAReturnRunsUnderTheBoxesItPassesAndIntoTheEarlierOne(t *testing.T) {
	t.Parallel()
	specs := []struct {
		id    string
		names []string
	}{
		{"a", []string{"Build", "Test", "Verify"}},
		{"b", []string{"Verify", "Build"}},
		{"c", []string{"Verify", "Test"}},
	}
	var tracked []FactoryTrackBelt
	for _, spec := range specs {
		base := DefaultsStages(TriggerKindIssues, DefaultsBuiltInAgents(), true)
		stages := make([]Stage, 0, len(spec.names))
		for index, name := range spec.names {
			stage := base[index]
			stage.ID = fmt.Sprintf("%s-%d", spec.id, index)
			stage.Name = name
			stage.Route = nil
			stages = append(stages, stage)
		}
		belt := NewBeltFromTrigger(BeltFromTriggerArgs{ID: spec.id, Name: spec.id, Trigger: Trigger{Kind: TriggerKindManual}, Stages: stages})
		tracked = append(tracked, FactoryTrackBelt{ID: belt.ID, Name: belt.Name, Layout: belt.Layout()})
	}
	track := NewFactoryTrack(tracked)
	if len(track.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(track.Returns))
	}
	layout := NewFloorLayout(track, OverviewBoxesMetrics(), nil, func(FactoryTrackStage) float64 { return 140 })
	if len(layout.Lanes) != 2 {
		t.Fatalf("lanes = %d, want 2", len(layout.Lanes))
	}
	metrics := OverviewBoxesMetrics()
	for _, lane := range layout.Lanes {
		from, to := layout.Box(lane.From), layout.Box(lane.To)
		if from == nil || to == nil {
			t.Fatal("a lane's ends are not placed")
		}
		if lane.Right != from.X+layout.Column/2 {
			t.Errorf("lane right = %v, want %v", lane.Right, from.X+layout.Column/2)
		}
		if lane.Left != to.X-layout.Column/2 {
			t.Errorf("lane left = %v, want %v", lane.Left, to.X-layout.Column/2)
		}
		for _, cell := range layout.Placed {
			if !isStage(cell.Node) || cell.X < to.X || cell.X > from.X {
				continue
			}
			if !(lane.LabelY >= cell.Top+cell.Height || lane.LabelY <= cell.Top) {
				t.Errorf("lane at %v runs through box %v (top %v, height %v)", lane.LabelY, cell.Node, cell.Top, cell.Height)
			}
		}
		if lane.LabelY >= layout.Height {
			t.Errorf("lane label %v lies below the layout's height %v", lane.LabelY, layout.Height)
		}
	}
	// Both go back to Build from Verify's column or before, so they overlap and stack.
	if math.Abs(layout.Lanes[0].LabelY-layout.Lanes[1].LabelY) < metrics.Lane {
		t.Errorf("lane rows %v and %v are closer than the lane spacing %v", layout.Lanes[0].LabelY, layout.Lanes[1].LabelY, metrics.Lane)
	}
}
