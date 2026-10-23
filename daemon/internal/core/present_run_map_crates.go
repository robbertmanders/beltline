//

package core

import "slices"

// MapCrate is a crate the run's map draws: the run itself, or one unfinished leg of a split run.
// X and Y are the crate's centre in points; Y is already lifted above its track.
type MapCrate struct {
	ID        string  `json:"id"`
	Reference string  `json:"reference"`
	State     string  `json:"state"`
	StageID   *string `json:"stageId,omitzero"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
}

// mapCrateLift is how far a crate rides above its track, measured from the track's centre.
const mapCrateLift = 11

// RunCrates is the crates the run's map draws, given the run's taken routes and the track
// thickness: one per unfinished leg of a split run, else the run's own crate. A run on a stage
// sits on its track; a finished run, or one with no stage, rides just above it.
func (g MapGeometry) RunCrates(run Run, routes map[string][]string, thickness float64) []MapCrate {
	if run.IsSplit() {
		var crates []MapCrate
		for _, leg := range run.Legs {
			if leg.IsFinished() || leg.StageID == nil {
				continue
			}
			point := g.Crate(leg.StageID, false, routes)
			crates = append(crates, MapCrate{
				ID:        leg.ID,
				Reference: run.Subject.Ref(),
				State:     string(leg.State),
				StageID:   leg.StageID,
				X:         point.X,
				Y:         point.Y - thickness/2 - mapCrateLift,
			})
		}
		return crates
	}
	finished := run.State.Completed()
	stageID := run.StageID
	point := g.Crate(&stageID, finished, routes)
	onStage := !finished && g.hasCellFor(run.StageID)
	crate := MapCrate{
		ID:        "main",
		Reference: run.Subject.Ref(),
		State:     string(run.State),
		X:         point.X,
		Y:         point.Y - thickness/2 - mapCrateLift,
	}
	if onStage {
		crate.Y = point.Y
	}
	if run.StageID != "" {
		crate.StageID = &stageID
	}
	return []MapCrate{crate}
}

// hasCellFor reports whether a stage has a cell on the map.
func (g MapGeometry) hasCellFor(stageID string) bool {
	return slices.ContainsFunc(g.Cells, func(cell MapGeometryPlaced) bool { return cell.Stage.ID == stageID })
}
