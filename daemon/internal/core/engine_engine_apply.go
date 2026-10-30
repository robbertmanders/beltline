// The outcome application, the
// take of routes, the starts of a leg and the replays of closed attempts.

package core

import (
	"slices"
	"strings"
	"time"
)

// engineAnyStage reports whether any of ids is a stage of the run.
func engineAnyStage(run Run, ids []string) bool {
	return slices.ContainsFunc(ids, func(id string) bool { return run.StageIndexOf(id) >= 0 })
}

// engineTake starts one leg per named route, each entering its route's first
// stage.
func engineTake(run *Run, routes []string, from string, note *string, now time.Time) []Effect {
	if run.Layout().Nested() {
		return engineNestedTake(run, routes, from, note, now)
	}
	layout := run.Layout()
	if len(routes) == 0 {
		return nil
	}
	fork := layout.ForkWithRoute(routes[0])
	if fork == nil {
		return nil
	}
	forkID := fork.ID
	span := layout.Span(*fork)
	if span == nil {
		return nil
	}
	// The belt's own route order decides the order of the legs.
	var taken []Route
	for _, route := range fork.Routes {
		if slices.Contains(routes, route.ID) {
			taken = append(taken, route)
		}
	}
	if len(taken) == 0 {
		return nil
	}
	ids := make([]string, 0, len(taken))
	for _, route := range taken {
		ids = append(ids, route.ID)
	}
	engineTaken(run)[forkID] = ids
	var legs []Leg
	for _, route := range taken {
		// A route with no stages is a leg that is finished the moment it is taken.
		var first *string
		if on := layout.StagesOn(route.ID); len(on) > 0 {
			first = new(on[0].ID)
		} else if stage, ok := arraySafe(layout.Stages, span.Upper+1); ok {
			first = new(stage.ID)
		}
		// One route is no split: the leg goes on as itself, on the run's
		// branch and in its folder. Only routes taken together get legs of their own.
		id := "r:" + route.ID
		if len(taken) == 1 {
			id = from
		}
		legs = append(legs, Leg{ID: id, Route: new(route.ID), StageID: first, State: RunStateQueued, Note: CopyPointer(note)})
	}
	if at := engineLegIndex(run.Legs, from); at >= 0 {
		updated := make([]Leg, 0, len(run.Legs)-1+len(legs))
		updated = append(updated, run.Legs[:at]...)
		updated = append(updated, legs...)
		updated = append(updated, run.Legs[at+1:]...)
		run.Legs = updated
	} else {
		run.Legs = append(run.Legs, legs...)
	}
	var effects []Effect
	for _, leg := range legs {
		effects = append(effects, engineStartLeg(run, leg.ID, leg.StageID, note, now)...)
	}
	return effects
}

// engineLegIndex is the position of the leg with id in legs, or -1.
func engineLegIndex(legs []Leg, id string) int {
	for i := range legs {
		if legs[i].ID == id {
			return i
		}
	}
	return -1
}

// engineStartAtBeginning starts a run at its first stage. A fork at the start of
// the belt has no preceding outcome to choose its route, so every route starts
// through the usual split and join behaviour, except one that feeds in: only a
// jump reaches that.
func engineStartAtBeginning(run *Run, note *string, now time.Time) []Effect {
	layout := run.Layout()
	if pieces := layout.Pieces(nil); len(pieces) > 0 && pieces[0].Fork != nil {
		var routes []string
		for _, route := range pieces[0].Fork.Routes {
			if !route.Feeds {
				routes = append(routes, route.ID)
			}
		}
		return engineTake(run, routes, "main", note, now)
	}
	return engineGo(run, layout.First(), note, now)
}

// engineStartLeg puts a leg into its route, or ships it at once when the route
// has no stage to work at (at nil).
func engineStartLeg(run *Run, leg string, at *string, note *string, now time.Time) []Effect {
	if at == nil {
		engineSet(run, &leg, func(l *Leg) {
			l.State = RunStateDone
			l.StageID = nil
			l.Note = nil
		})
		return nil
	}
	return engineEnter(run, *at, &leg, note, now)
}

// engineFail ends a failed agent session on leg with detail.
func engineFail(run *Run, leg string, stage Stage, detail string, now time.Time) []Effect {
	fallbackKey := stage.ID + ":fallback"
	if stage.Fallback == FallbackRetry && engineUses(run)[fallbackKey] < 1 {
		engineUses(run)[fallbackKey]++
		engineCloseAttempt(run, &leg, now, func(a *Attempt) {
			a.Tone = new(ToneProblem)
			a.Detail = new(detail)
			a.Route = new("Failed: running it again")
		})
		return engineEnter(run, stage.ID, &leg, nil, now)
	}
	if stage.Fallback == FallbackDrop {
		engineCloseAttempt(run, &leg, now, func(a *Attempt) {
			a.Tone = new(ToneProblem)
			a.Detail = new(detail)
			a.Route = new("Failed: dropped")
		})
		return engineDropLeg(run, &leg, stage.Name+" failed", now)
	}
	engineCloseAttempt(run, &leg, now, func(a *Attempt) {
		a.Tone = new(ToneProblem)
		a.Detail = new(detail)
		a.Route = new("Failed: held for you")
	})
	return engineHoldLeg(run, &leg, stage.Name+" failed", nil, now, true)
}

// engineReplayClosedAttempt sets a fresh session at an agent stage that already
// ran to read that stage's latest closed attempt. Called only after goto, route
// or next has landed: enter clears replays, and a retry, a failure fallback, an
// edit that starts the belt again, or a resume that asks for a fresh session
// must not put it back.
func engineReplayClosedAttempt(run *Run, leg *string) {
	if leg == nil {
		return
	}
	current := run.LegByID(leg)
	if current == nil || current.State != RunStateQueued || current.StageID == nil {
		return
	}
	stageID := *current.StageID
	index := run.StageIndexOf(stageID)
	if index < 0 || run.Stages[index].Kind() != StageKindAgent {
		return
	}
	source := engineReplaySource(*run, stageID, *leg)
	if source == nil {
		return
	}
	sourceID := source.ID
	engineSet(run, leg, func(l *Leg) { l.Replays = new(sourceID) })
}

// engineReplaySource is the closed attempt leg should replay at stage. The leg's
// own attempts win. A stage on a route may have been worked under an earlier leg
// (main, or r:<route>), and that work counts once this leg is the one that landed
// on the stage.
func engineReplaySource(run Run, stage string, leg string) *Attempt {
	onLeg := run.ClosedAttempts(stage, &leg)
	var candidates []Attempt
	if len(onLeg) > 0 {
		candidates = onLeg
	} else {
		index := run.StageIndexOf(stage)
		if index < 0 || run.Stages[index].Route == nil {
			return nil
		}
		routeLeg := "r:" + *run.Stages[index].Route
		for _, attempt := range run.ClosedAttempts(stage, nil) {
			if attempt.Leg == nil || *attempt.Leg == routeLeg {
				candidates = append(candidates, attempt)
			}
		}
	}
	// The first of equal maxima is kept.
	var best *Attempt
	for i := range candidates {
		if best == nil || candidates[i].Number > best.Number {
			best = &candidates[i]
		}
	}
	return best
}

// engineSessionStarted is the sessionStarted event: a queued agent leg's session
// begins, and its attempt records the session it runs in.
func engineSessionStarted(run *Run, event Event, now time.Time) []Effect {
	at := engineLeg(*run, event.Leg)
	stage := at.Stage(*run)
	if at.State != RunStateQueued || stage == nil || stage.Kind() != StageKindAgent {
		return nil
	}
	legID := at.ID
	attempt := engineNewAttempt(*run, &legID, at.StageID, now, event.Folder)
	attempt.Session = new(event.Session)
	attempt.SessionID = new(event.SessionID)
	if continued := run.AttemptByID(at.Continues); continued != nil {
		attempt.Continues = new(continued.ID)
		attempt.AgentName = continued.AgentSessionName()
	}
	attempt.Replays = CopyPointer(at.Replays)
	// The merge note belongs to the leg the merge was for.
	if run.MergeNoteLeg == nil || *run.MergeNoteLeg == at.ID {
		attempt.Detail = CopyPointer(run.MergeNote)
		run.MergeNote = nil
		run.MergeNoteLeg = nil
	}
	run.Attempts = append(run.Attempts, attempt)
	engineSet(run, &legID, func(l *Leg) {
		l.Note = nil
		l.Continues = nil
		l.Replays = nil
		l.State = RunStateRunning
	})
	return nil
}

// engineSessionFinished is the sessionFinished event: a running agent session
// ended with output, and its outcome line (or the failure) decides what comes
// next.
func engineSessionFinished(run *Run, event Event, now time.Time) []Effect {
	at := engineLeg(*run, event.Leg)
	stage := at.Stage(*run)
	if at.State != RunStateRunning || stage == nil || stage.Kind() != StageKindAgent {
		return nil
	}
	// A late result from a session that is no longer the leg's open attempt
	// (Send to or Abort ended it) changes nothing.
	if event.Session != "" && !engineOpenSessionIs(*run, &at.ID, event.Session) {
		return nil
	}
	if EngineStaleInterruptedTurn(*run, &at.ID) {
		return nil
	}
	if event.Output != nil {
		if outcome := OutcomeParserParse(*event.Output, stage.Outcomes); outcome != nil {
			return engineApply(run, at.ID, *stage, *outcome, event.Output, now)
		}
	}
	var why string
	switch {
	case event.Detail != nil:
		why = *event.Detail
	case event.Output == nil:
		why = "the session ended without writing " + stage.Writes()
	default:
		why = "no known outcome in " + stage.Writes()
	}
	return engineFail(run, at.ID, *stage, why, now)
}

// engineOpenSessionIs reports whether leg's open attempt runs session.
func engineOpenSessionIs(run Run, leg *string, session string) bool {
	index := engineOpenAttemptIndex(run, leg)
	return index != nil && run.Attempts[*index].Session != nil && *run.Attempts[*index].Session == session
}

// engineDecided is the decided event: an approval that is waiting for you takes
// the outcome you picked, with the note you wrote.
func engineDecided(run *Run, event Event, now time.Time) []Effect {
	at := engineLeg(*run, event.Leg)
	stage := at.Stage(*run)
	if at.State != RunStateWaiting || stage == nil || stage.Kind() != StageKindGate {
		return nil
	}
	outcome := stage.Outcome(event.Outcome)
	if outcome == nil {
		return nil
	}
	note := event.Note
	if stage.Type == StageTypePrompt && stage.NeedsNote(*outcome) {
		// The prompt becomes the run's task, which every later brief carries, so it
		// is not passed on as a note too.
		task := strings.TrimSpace(ValueOf(event.Note))
		if task == "" {
			return nil
		}
		run.Task = &task
		run.Sharing = event.Sharing
		if !run.Sharing.Known() {
			run.Sharing = *stage.SourceSharing()
		}
		note = nil
	}
	if event.Note != nil && *event.Note != "" {
		detail := *event.Note
		engineCloseAttempt(run, &at.ID, now, func(a *Attempt) { a.Detail = new(detail) })
	}
	return engineApply(run, at.ID, *stage, *outcome, note, now)
}
