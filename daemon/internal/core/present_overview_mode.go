package core

// OverviewMode is a look the Overview page has: the summary you land on, and the Floor.
type OverviewMode string

const (
	OverviewModeSummary OverviewMode = "summary"
	OverviewModeFloor   OverviewMode = "floor"
)

// AllOverviewMode lists the cases of OverviewMode in declaration order (CaseIterable).
var AllOverviewMode = []OverviewMode{OverviewModeSummary, OverviewModeFloor}

// OverviewModeLanding is the look every launch opens on, whatever was on screen when the
// app was last closed. The floor is a look you switch to while you are there, not the
// place the window comes up in, so it isn't kept.
const OverviewModeLanding = OverviewModeSummary
