// layout itself is in present_box_layout.go).
//
// A belt drawn as stage boxes, in points: the trigger and every stage in a
// column of its own, a fork's routes on rows below the line, and hooked
// tracks between them (straight, or turning on rounded corners). The run page, the belt editor and the Market's
// package preview all draw belts this way; each adds its own words and
// states to the nodes by id. The app only draws it.

package core

// BoxMapMetrics are the sizes a box map is laid out with, in points.
type BoxMapMetrics struct {
	NodeWidth  float64
	NodeHeight float64
	// Gap is the space between two columns, where the tracks run.
	Gap float64
	// RowHeight is from one route's row to the next.
	RowHeight float64
	PadX      float64
	// PadTop is room above the first row, for status chips or a fork's caption.
	PadTop    float64
	PadBottom float64
	Corner    float64
	// Lead is the columns of bare track a route runs from its fork's spine before its first box, where its
	// name sits on the track (the run page). Zero puts the name above the route's first box instead.
	Lead float64
}

// BoxMapMetricsRun is the run page: short boxes with a status chip above each, and each route's
// name on a lead of track.
var BoxMapMetricsRun = BoxMapMetrics{NodeWidth: 148, NodeHeight: 52, Gap: 60, RowHeight: 100, PadX: 2, PadTop: 34, PadBottom: 4, Corner: 10, Lead: 1}

// BoxMapMetricsEditor is the belt editor: tall boxes with room for outputs, a + between stages and fork areas.
var BoxMapMetricsEditor = BoxMapMetrics{NodeWidth: 132, NodeHeight: 100, Gap: 44, RowHeight: 124, PadX: 30, PadTop: 60, PadBottom: 14, Corner: 10, Lead: 0}

// BoxMapMetricsPreview is the Market's read-only preview of a package's belt.
var BoxMapMetricsPreview = BoxMapMetrics{NodeWidth: 132, NodeHeight: 100, Gap: 44, RowHeight: 124, PadX: 30, PadTop: 52, PadBottom: 10, Corner: 10, Lead: 0}

// BoxMapKind is what a box is.
type BoxMapKind string

const (
	BoxMapKindTrigger  BoxMapKind = "trigger"
	BoxMapKindAgent    BoxMapKind = "agent"
	BoxMapKindApproval BoxMapKind = "approval"
	BoxMapKindWatch    BoxMapKind = "watch"
)

// BoxMapNode is one box of the map: the trigger or a stage.
type BoxMapNode struct {
	// ID is the stage's id, or `trigger`.
	ID   string     `json:"id"`
	Kind BoxMapKind `json:"kind"`
	// Number is the stage's number as drawn (its column); 0 for the trigger.
	Number int `json:"number"`
	// Fork and Route are the fork and route the stage sits on, innermost first; nil on the line.
	Fork   *string   `json:"fork,omitzero"`
	Route  *string   `json:"route,omitzero"`
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	Width  float64   `json:"width"`
	Height float64   `json:"height"`
	Icon   StageIcon `json:"icon"`
	// Tint is the box's icon colour; nil for the trigger, which is drawn dark.
	Tint *Tint `json:"tint,omitzero"`
}

// BoxMapEdge is one track between two boxes.
type BoxMapEdge struct {
	From string  `json:"from"`
	To   string  `json:"to"`
	Path MapPath `json:"path"`
	// Straight is whether the edge runs straight along one row.
	Straight bool `json:"straight"`
	// Route is the route whose track this is: a route's entry or exit, or an empty route's whole row. Nil on the line.
	Route *string `json:"route,omitzero"`
}

// BoxMapForkArea is a fork's area: the dashed box around its routes in the editor.
type BoxMapForkArea struct {
	ID     string  `json:"id"`
	Routes int     `json:"routes"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// BoxMapRoutePill is a route's name on its row, where it leaves the fork's start.
type BoxMapRoutePill struct {
	Fork  string `json:"fork"`
	Route string `json:"route"`
	Name  string `json:"name"`
	// X and Y are the pill's centre: on the route's lead track, halfway from the fork's spine to its first box
	// (on the spine when there is no lead), and its row's centre.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// StageIDs are the stages on the route, nested forks included.
	StageIDs []string `json:"stageIds"`
	Feeds    bool     `json:"feeds"`
}

// BoxMapInsert is where a + adds a stage: in front of a box (the gap before it, on its row), at the end of a route
// (the gap after its last box, or in the middle of an empty route's slot) or past the last box of the line.
type BoxMapInsert struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// After and Before are the box before it and the box after it, where there are any; Before is nil at the end of a list.
	After  *string `json:"after,omitzero"`
	Before *string `json:"before,omitzero"`
	// Route is the route a stage added here joins; nil on the line.
	Route *string `json:"route,omitzero"`
	// AddAt is the index in the belt's stages where the stage goes, as the add command takes it.
	AddAt int `json:"addAt"`
}

// BoxMap is a belt drawn as stage boxes, in points.
type BoxMap struct {
	Width   float64           `json:"width"`
	Height  float64           `json:"height"`
	Nodes   []BoxMapNode      `json:"nodes"`
	Edges   []BoxMapEdge      `json:"edges"`
	Forks   []BoxMapForkArea  `json:"forks"`
	Routes  []BoxMapRoutePill `json:"routes"`
	Inserts []BoxMapInsert    `json:"inserts"`
}

// Node is the node with id, or nil.
func (m BoxMap) Node(id string) *BoxMapNode {
	for index := range m.Nodes {
		if m.Nodes[index].ID == id {
			node := m.Nodes[index]
			return &node
		}
	}
	return nil
}
