package core

// StageMenuOption is one choice in a stage menu list.
type StageMenuOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// StageMenuOptionNew is an option from its parts.
func StageMenuOptionNew(id, label string) StageMenuOption {
	return StageMenuOption{ID: id, Label: label}
}

// StageMenuPlacement is where the stage sits on its fork, and the routes it can move to.
type StageMenuPlacement struct {
	// Off is what choosing no route means: off the fork, onto the line or the enclosing route.
	Off    string            `json:"off"`
	Routes []StageMenuOption `json:"routes"`
}

// StageMenuGotoRoute is a route of a fork a jump comes back into, with the stages a run
// that took it can go to.
type StageMenuGotoRoute struct {
	Route  StageMenuOption   `json:"route"`
	Stages []StageMenuOption `json:"stages"`
}

// StageMenuGotoTarget is one stage a `goto` can name first, with what choosing it adds.
type StageMenuGotoTarget struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Counterparts are the stages choosing it adds for every other route of a fork the
	// jump comes back into.
	Counterparts []string `json:"counterparts"`
	// Others are those routes, each with the stages a run that took it can be sent to instead.
	Others []StageMenuGotoRoute `json:"others"`
}

// StageMenuRunner is one runner in the menu, with the thinking levels it takes.
type StageMenuRunner struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Thinking are the thinking levels this runner takes, in order.
	Thinking []StageMenuOption `json:"thinking"`
	// ThinkingNote is why Thinking is off, when the runner takes none.
	ThinkingNote *string `json:"thinkingNote,omitzero"`
}

// StageMenuPreview is the preview under the outcomes: a title and its lines.
type StageMenuPreview struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

// StageMenu is what the stage form offers for one stage of a draft, computed from the draft
// the way the native form computes it: the route it can move to, where a
// `goto` can go and which counterparts each jump takes, the routes a `route`
// outcome can take, the outcome actions, workspaces, watch targets, runners
// and the preview under the outcomes. The editor renders these and computes none.
type StageMenu struct {
	Stage string `json:"stage"`
	// Number is the stage's number on the belt's drawing, from 1.
	Number         int                 `json:"number"`
	Kind           string              `json:"kind"`
	CanMoveEarlier bool                `json:"canMoveEarlier"`
	CanMoveLater   bool                `json:"canMoveLater"`
	Placement      *StageMenuPlacement `json:"placement,omitzero"`
	Workspaces     []StageMenuOption   `json:"workspaces"`
	WatchTargets   []StageMenuOption   `json:"watchTargets"`
	// WatchEvents are the `when` values the stage's watch can report, for a watch stage.
	WatchEvents []StageMenuOption `json:"watchEvents"`
	WatchNote   *string           `json:"watchNote,omitzero"`
	Actions     []StageMenuOption `json:"actions"`
	// RoutesAfter are the routes of the fork right after the stage, by their full path.
	RoutesAfter []StageMenuOption `json:"routesAfter"`
	// RoutePaths are every route of the belt by its full path, to name a saved route that is unavailable here.
	RoutePaths  []StageMenuOption     `json:"routePaths"`
	GotoTargets []StageMenuGotoTarget `json:"gotoTargets"`
	Runners     []StageMenuRunner     `json:"runners"`
	// DefaultRunner is the runner a stage without one runs on.
	DefaultRunner  string           `json:"defaultRunner"`
	OutcomesIntro  string           `json:"outcomesIntro"`
	WhenTitle      string           `json:"whenTitle"`
	Preview        StageMenuPreview `json:"preview"`
	AlternatesNote string           `json:"alternatesNote"`
}

// Copy used by StageMenu.make. Literal text is exact.
const (
	stageMenuPathSeparator = " / "
)

// StageMenuMake is the menu for stageID on belt, or nil when the belt has no such stage.
func StageMenuMake(belt Belt, stageID string) *StageMenu {
	var stage *Stage
	for i := range belt.Stages {
		if belt.Stages[i].ID == stageID {
			stage = &belt.Stages[i]
			break
		}
	}
	if stage == nil {
		return nil
	}
	layout := belt.Layout()
	kind := "Agent stage"
	switch stage.Kind() {
	case StageKindGate:
		kind = "Approval"
	case StageKindWatch:
		kind = "Watcher"
	}
	watchEvents := []StageMenuOption{}
	var watchNote *string
	if target := stage.WatchTarget(); target != nil {
		for _, event := range target.Events() {
			watchEvents = append(watchEvents, StageMenuOptionNew(event.When, event.Label))
		}
		note := stageMenuWatchNote(*target)
		watchNote = &note
	}
	whenTitle := "Happens when"
	switch stage.Kind() {
	case StageKindAgent:
		whenTitle = "Pick it when"
	case StageKindGate:
		whenTitle = "Button"
	}
	watchTargets := []StageMenuOption{}
	if stage.Kind() == StageKindWatch {
		watchTargets = stageMenuWatchTargets(*stage, belt)
	}
	routesAfter := []StageMenuOption{}
	for _, route := range layout.RoutesAfter(stage.ID) {
		routesAfter = append(routesAfter, StageMenuOptionNew(route.ID, stageMenuRoutePath(route.ID, layout)))
	}
	routePaths := []StageMenuOption{}
	for _, fork := range belt.Forks {
		for _, route := range fork.Routes {
			routePaths = append(routePaths, StageMenuOptionNew(route.ID, stageMenuRoutePath(route.ID, layout)))
		}
	}
	runners := []StageMenuRunner{}
	for _, runner := range AllBackend {
		supported := runner.SupportedThinking()
		thinking := []StageMenuOption{}
		for _, level := range AllThinking {
			if _, ok := supported[level]; ok {
				thinking = append(thinking, StageMenuOptionNew(string(level), level.Label()))
			}
		}
		runners = append(runners, StageMenuRunner{ID: string(runner), Label: runner.Label(), Thinking: thinking, ThinkingNote: runner.ThinkingNote()})
	}
	return &StageMenu{
		Stage:          stage.ID,
		Number:         layout.Number(stage.ID),
		Kind:           kind,
		CanMoveEarlier: stageMenuCanShift(belt, stage.ID, -1),
		CanMoveLater:   stageMenuCanShift(belt, stage.ID, 1),
		Placement:      stageMenuPlacement(*stage, belt),
		Workspaces:     stageMenuWorkspaces(*stage, belt),
		WatchTargets:   watchTargets,
		WatchEvents:    watchEvents,
		WatchNote:      watchNote,
		Actions:        stageMenuActions(*stage, belt),
		RoutesAfter:    routesAfter,
		RoutePaths:     routePaths,
		GotoTargets:    stageMenuGotoTargets(*stage, belt),
		Runners:        runners,
		DefaultRunner:  string(BackendPi),
		OutcomesIntro:  stageMenuIntro(*stage),
		WhenTitle:      whenTitle,
		Preview:        stageMenuPreview(*stage),
		AlternatesNote: "Each fresh session takes the model fewest of this stage's running sessions use, drawing at random between equals. A run that comes back to the stage keeps the model it had.",
	}
}
