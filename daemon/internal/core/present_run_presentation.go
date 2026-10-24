// Pure details the Run page and API give to a client for every stage. Browser
// selection stays local: this projection always identifies the open or latest
// attempt as the default without saving a preference. The API sends these types
// as they are.

package core

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// RunPresentationAttemptEntryContent is what an attempt entry shows.
type RunPresentationAttemptEntryContent string

const (
	RunPresentationAttemptEntryContentRunning          RunPresentationAttemptEntryContent = "running"
	RunPresentationAttemptEntryContentDecision         RunPresentationAttemptEntryContent = "decision"
	RunPresentationAttemptEntryContentHistoricalOutput RunPresentationAttemptEntryContent = "historicalOutput"
	RunPresentationAttemptEntryContentEmpty            RunPresentationAttemptEntryContent = "empty"
)

// RunPresentationAttemptEntry is one attempt in a stage panel.
type RunPresentationAttemptEntry struct {
	ID             string                             `json:"id"`
	Number         int                                `json:"number"`
	Caption        string                             `json:"caption"`
	Content        RunPresentationAttemptEntryContent `json:"content"`
	RunningAttempt *int                               `json:"runningAttempt,omitzero"`
	Outcome        *string                            `json:"outcome,omitzero"`
	Tone           *Tone                              `json:"tone,omitzero"`
	Route          *string                            `json:"route,omitzero"`
	Note           *string                            `json:"note,omitzero"`
	Detail         *string                            `json:"detail,omitzero"`
	IsOpen         bool                               `json:"isOpen"`
}

// RunPresentationStagePanel is one stage on the run page.
type RunPresentationStagePanel struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Number int       `json:"number"`
	Kind   StageKind `json:"kind"`
	// Step goes out as the panel's step word (api.RunStageWire), which writes todo as notStarted.
	Step             StatusStep                    `json:"-"`
	Status           StageStatus                   `json:"status"`
	DefaultAttempt   *string                       `json:"defaultAttempt,omitzero"`
	Attempts         []RunPresentationAttemptEntry `json:"attempts"`
	EmptyMessage     *string                       `json:"emptyMessage,omitzero"`
	LiveWatchSummary *string                       `json:"liveWatchSummary,omitzero"`
	// Worker is the runner or agent label shown under the stage name.
	Worker *string `json:"worker,omitzero"`
}

// RunPresentationWatchNote is one live watch observation on a pull request.
type RunPresentationWatchNote struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Note      string `json:"note"`
}

// RunPresentationSnapshot is everything the run page shows for one run.
type RunPresentationSnapshot struct {
	RunID        string
	DefaultStage string
	Metadata     string
	FirstVerdict *string
	Spend        Spend
	CostLine     *string
	SessionCount int
	Links        []RunLinksTarget
	WatchNotes   []RunPresentationWatchNote
	ErasureCopy  string
	PauseCopy    *string
	Stages       []RunPresentationStagePanel
	Legs         []StatusLegRow
	Actions      []RunAction
}

// Copy used by RunPresentation. Literal text is exact.
const (
	runPresentationWaitingFor    = "Waiting for "
	runPresentationMergedSuffix  = " to be merged."
	runPresentationListSeparator = ", "
)

// RunPresentationCaption is the caption over the belt: where the run is now, or what it finished as.
func RunPresentationCaption(run Run, ambiguous map[string]struct{}) string {
	if !run.State.Completed() {
		name := ""
		if stage := run.stageRef(); stage != nil {
			name = stage.Name
		}
		return "Now at " + name
	}
	word := runCompletedWord(run)
	if len(run.PullRequests) == 0 {
		return word
	}
	labels := make([]string, 0, len(run.PullRequests))
	for _, pull := range run.PullRequests {
		labels = append(labels, RepoLabelPullRequest(pull.Repo, pull.Number, ambiguous))
	}
	return word + " as " + strings.Join(labels, runPresentationListSeparator)
}

// RunSessionCount is how many of the run's attempts ran an agent session: the count the
// Sessions button shows. An attempt counts only with a non-empty session name.
func RunSessionCount(run Run) int {
	count := 0
	for _, attempt := range run.Attempts {
		if attempt.Session != nil && *attempt.Session != "" {
			count++
		}
	}
	return count
}

// RunPresentationMetadata is the belt version and number of stages shown above the map.
func RunPresentationMetadata(run Run) string {
	builds := 0
	for _, attempt := range run.Attempts {
		for _, stage := range run.Stages {
			if stage.ID == attempt.StageID {
				if stage.ChangesCode() {
					builds++
				}
				break
			}
		}
	}
	text := "Belt v" + strconv.Itoa(run.BeltVersion) + " · "
	if run.OnShadow() {
		text += "shadow · "
	}
	text += strconv.Itoa(len(run.Stages)) + " stages"
	if builds > 1 {
		text += " · build attempt " + strconv.Itoa(builds)
	}
	return text
}

// RunPresentationPauseCopy is the run page's pause and continuation explanation; nil when
// the run has neither a pause nor a queued continuation.
func RunPresentationPauseCopy(run Run) *string {
	continuing := slices.ContainsFunc(run.Attempts, func(attempt Attempt) bool {
		return attempt.IsOpen() && attempt.Interruption != nil && attempt.Interruption.Continuation != nil &&
			attempt.Interruption.Continuation.Status == AttemptInterruptionContinuationStatusQueued
	})
	if run.Pause == nil && !continuing {
		return nil
	}
	if run.Pause == nil {
		failed := slices.ContainsFunc(run.Attempts, func(attempt Attempt) bool {
			return attempt.Interruption != nil && attempt.Interruption.Continuation != nil &&
				attempt.Interruption.Continuation.Status == AttemptInterruptionContinuationStatusFailed
		})
		if failed {
			var reasons []string
			for _, attempt := range run.Attempts {
				if attempt.Interruption != nil && attempt.Interruption.Continuation != nil &&
					attempt.Interruption.Continuation.Status == AttemptInterruptionContinuationStatusFailed &&
					attempt.Interruption.Continuation.Reason != nil {
					reasons = append(reasons, *attempt.Interruption.Continuation.Reason)
				}
			}
			return new(strings.Join(reasons, " "))
		}
		return new(RunPauseExplanation)
	}
	pause := run.Pause
	reason := ValueOf(pause.Reason)
	switch pause.Phase {
	case RunPausePhasePausing:
		return new("Pausing…")
	case RunPausePhaseIncomplete:
		return new("Pause incomplete. " + reason)
	case RunPausePhaseReconcileFailed:
		if pause.Reason != nil {
			return new(*pause.Reason)
		}
		return new("Could not read the source. Retry reads it again.")
	default:
		return new(RunPauseExplanation)
	}
}

// RunPresentationEmptyMessage is the empty message shown for a stage that has no saved
// attempt.
func RunPresentationEmptyMessage(stage Stage, run Run, step StatusStep) string {
	if step == StatusStepQueued {
		return "Queued. It starts when " + stage.Name + " is free."
	}
	var then string
	switch stage.Kind() {
	case StageKindGate:
		then = "You decide here when it does."
	case StageKindWatch:
		if statusIsTicketWatch(stage) {
			then = "Beltline watches the " + run.Subject.Kind.Noun() + " and the tickets under it when it does."
		} else {
			then = "Beltline watches its pull request when it does."
		}
	default:
		then = "Its output appears here when " + runPresentationRunnerLabel(run, stage) + " is done."
	}
	return run.Subject.Ref() + " hasn't reached " + stage.Name + " yet. " + then
}

// RunPresentationWorker is who works stage for run, as the run page lists it under the
// stage: on a stage that spreads its sessions, the runner the run took.
func RunPresentationWorker(stage Stage, run Run) string {
	switch stage.Kind() {
	case StageKindGate:
		return "you"
	case StageKindWatch:
		return "github"
	default:
		return LowercasedFull(runPresentationRunnerLabel(run, stage))
	}
}

// RunPresentationWatchNotes are the current watch observations of run's pull requests.
// Completed watch attempts retain only saved detail in their panel and do not use these
// live notes.
func RunPresentationWatchNotes(run Run, ambiguous map[string]struct{}) []RunPresentationWatchNote {
	notes := []RunPresentationWatchNote{}
	for _, pull := range run.PullRequests {
		note := WatchNotesSeen(pull, false)
		if note == nil {
			continue
		}
		notes = append(notes, RunPresentationWatchNote{
			ID:        pull.Repo + "#" + strconv.Itoa(pull.Number),
			Reference: RepoLabelPullRequest(pull.Repo, pull.Number, ambiguous),
			Note:      *note,
		})
	}
	return notes
}

// RunPresentationLooking is a watch's line while no pull request is known yet: "Looking for
// the pull request from beltline/issue-142.". It is nil once the run has one.
func RunPresentationLooking(run Run) *string {
	if len(run.PullRequests) != 0 {
		return nil
	}
	branch := ValueOr(run.Branch, "the run's branch")
	line := "Looking for the pull request from " + branch + "."
	return &line
}

// RunPresentationWatchingSummary is the run page's live watch summary. Completed watch
// attempts do not use this value; they retain their saved attempt detail instead.
func RunPresentationWatchingSummary(run Run, stage Stage, ambiguous map[string]struct{}) string {
	if statusIsTicketWatch(stage) {
		var observation *TicketObservation
		if seen, ok := run.TicketObservations[stage.ID]; ok {
			observation = &seen
		}
		return TicketWatchSummary(observation, run.Subject.Ref())
	}
	if looking := RunPresentationLooking(run); looking != nil {
		return *looking
	}
	names := make([]string, 0, len(run.PullRequests))
	for _, pull := range run.PullRequests {
		names = append(names, RepoLabelPullRequest(pull.Repo, pull.Number, ambiguous))
	}
	joinedNames := strings.Join(names, runPresentationListSeparator)
	notes := []string{}
	for _, note := range RunPresentationWatchNotes(run, ambiguous) {
		notes = append(notes, note.Reference+": "+note.Note)
	}
	if len(notes) == 0 {
		return runPresentationWaitingFor + joinedNames + runPresentationMergedSuffix
	}
	return runPresentationWaitingFor + joinedNames + runPresentationMergedSuffix + " Right now: " + strings.Join(notes, runPresentationListSeparator) + "."
}

// RunPresentationProject is the run page's full projection for one run.
func RunPresentationProject(run Run, pausedBelt bool, pause Pause, now time.Time,
	ambiguousRepositories map[string]struct{}, actions []RunAction) RunPresentationSnapshot {
	in := runPresentationProjection{
		run:                   run,
		layout:                run.Layout(),
		pausedBelt:            pausedBelt,
		pause:                 pause,
		now:                   now,
		ambiguousRepositories: ambiguousRepositories,
	}
	panels := make([]RunPresentationStagePanel, 0, len(run.Stages))
	for _, stage := range run.Stages {
		panels = append(panels, in.stagePanel(stage))
	}
	spend := CostSpendOf(run)
	return RunPresentationSnapshot{
		RunID:        run.ID,
		DefaultStage: run.StageID,
		Metadata:     RunPresentationMetadata(run),
		FirstVerdict: runPresentationFirstVerdict(run),
		Spend:        spend,
		CostLine:     CostRunLine(spend),
		SessionCount: RunSessionCount(run),
		Links:        RunLinksTargets(run.Subject, run.PullRequests, ambiguousRepositories),
		WatchNotes:   in.watchNotes(),
		ErasureCopy:  ErasureCopyMessage(run),
		PauseCopy:    RunPresentationPauseCopy(run),
		Stages:       panels,
		Legs:         StatusLegs(run, pausedBelt, pause, now),
		Actions:      actions,
	}
}

// runPresentationRunnerLabel is the label of the runner the run's stage works on, with the
// Pi default.
func runPresentationRunnerLabel(run Run, stage Stage) string {
	runner := BackendPi
	if settings := ModelPoolCurrent(run, stage); settings.Runner != nil {
		runner = *settings.Runner
	}
	return runner.Label()
}
