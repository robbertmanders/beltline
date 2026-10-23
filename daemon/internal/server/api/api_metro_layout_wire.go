package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// MetroLayoutWire is the metro layout's geometry, in points: its lines and
// stations with what they derive, and its dots and labels as core lays them out.
type MetroLayoutWire struct {
	core.MetroLayout
	Lines    []MetroLayoutWireLine    `json:"lines"`
	Stations []MetroLayoutWireStation `json:"stations"`
}

// MetroLayoutWireLine is one belt's line with its id, its stretches as SVG path
// data, corners rounded, and the bars across its ends.
type MetroLayoutWireLine struct {
	core.MetroLayoutLine
	ID          string                `json:"id"`
	Paths       []string              `json:"paths"`
	ReturnPaths []string              `json:"returnPaths"`
	EndBars     []core.MetroLayoutBar `json:"endBars"`
}

// MetroLayoutWireStation is one station with the span of its stops, the belts
// that stop there and the box it covers.
type MetroLayoutWireStation struct {
	core.MetroLayoutStation
	Top    float64             `json:"top"`
	Bottom float64             `json:"bottom"`
	Belts  []string            `json:"belts"`
	Box    core.MetroLayoutBox `json:"box"`
}

// NewMetroLayoutWire builds the metro layout wire from the layout.
func NewMetroLayoutWire(layout core.MetroLayout) MetroLayoutWire {
	out := MetroLayoutWire{
		MetroLayout: layout,
		Lines:       make([]MetroLayoutWireLine, 0, len(layout.Lines)),
		Stations:    make([]MetroLayoutWireStation, 0, len(layout.Stations)),
	}
	for _, line := range layout.Lines {
		out.Lines = append(out.Lines, MetroLayoutWireLine{MetroLayoutLine: line, ID: line.ID(), Paths: line.Paths(),
			ReturnPaths: line.ReturnPaths(), EndBars: line.EndBars()})
	}
	for _, station := range layout.Stations {
		out.Stations = append(out.Stations, MetroLayoutWireStation{MetroLayoutStation: station, Top: station.Top(),
			Bottom: station.Bottom(), Belts: station.Belts(), Box: station.Box()})
	}
	return out
}
