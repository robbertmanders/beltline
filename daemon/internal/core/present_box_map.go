// A belt drawn as stage boxes, as a grid: the trigger and every stage in a
// column of its own, a fork's routes on rows above and below the line, and the
// tracks between them with the corners they turn. Columns and rows are cells,
// not points: the run page sizes the cells and draws the boxes and tracks in
// points (the layout itself is in present_box_layout.go).

package core

// boxMapLead is the columns of bare track a route runs from its fork's spine before its first box, where its
// name sits on the track.
const boxMapLead = 1

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
	Fork  *string `json:"fork,omitzero"`
	Route *string `json:"route,omitzero"`
	// Column is the box's column, from the left; Row its row's centre, in rows from the line (negative above it).
	Column int       `json:"column"`
	Row    float64   `json:"row"`
	Icon   StageIcon `json:"icon"`
	// Tint is the box's icon colour; nil for the trigger, which is drawn dark.
	Tint *Tint `json:"tint,omitzero"`
}

// BoxMapTurn is a corner a track turns: on the gap in front of Column, at Row.
type BoxMapTurn struct {
	Column int     `json:"column"`
	Row    float64 `json:"row"`
}

// BoxMapEdge is one track between two boxes: out of From's right edge, through the corners in Via, and into
// To's left edge.
type BoxMapEdge struct {
	From string       `json:"from"`
	To   string       `json:"to"`
	Via  []BoxMapTurn `json:"via"`
	// Route is the route whose track this is: a route's entry or exit, or an empty route's whole row. Nil on the line.
	Route *string `json:"route,omitzero"`
}

// BoxMapRoutePill is a route's name on its row, where it leaves the fork's start.
type BoxMapRoutePill struct {
	Fork  string `json:"fork"`
	Route string `json:"route"`
	Name  string `json:"name"`
	// Column is the route's lead, the column of bare track its name sits on; Row is its row's centre.
	Column int     `json:"column"`
	Row    float64 `json:"row"`
	// StageIDs are the stages on the route, nested forks included.
	StageIDs []string `json:"stageIds"`
	Feeds    bool     `json:"feeds"`
}

// BoxMap is a belt drawn as stage boxes, on a grid.
type BoxMap struct {
	// Columns is how many columns the map takes.
	Columns int               `json:"columns"`
	Nodes   []BoxMapNode      `json:"nodes"`
	Edges   []BoxMapEdge      `json:"edges"`
	Routes  []BoxMapRoutePill `json:"routes"`
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
