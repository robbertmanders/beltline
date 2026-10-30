package core

// EventKind is which case of Event a value is.
type EventKind string

const (
	EventKindSessionStarted   EventKind = "sessionStarted"
	EventKindSessionFinished  EventKind = "sessionFinished"
	EventKindDecided          EventKind = "decided"
	EventKindWatched          EventKind = "watched"
	EventKindTicketWatched    EventKind = "ticketWatched"
	EventKindSubjectChanged   EventKind = "subjectChanged"
	EventKindSubjectGone      EventKind = "subjectGone"
	EventKindBlocked          EventKind = "blocked"
	EventKindAdmissionBlocked EventKind = "admissionBlocked"
	// EventKindResume is a resume. Every caller must build it with EventResume,
	// which sets Earlier to true; a literal leaves Earlier false.
	EventKindResume                EventKind = "resume"
	EventKindParkRun               EventKind = "parkRun"
	EventKindUnparkRun             EventKind = "unparkRun"
	EventKindParkStopped           EventKind = "parkStopped"
	EventKindAnswered              EventKind = "answered"
	EventKindAbort                 EventKind = "abort"
	EventKindDrop                  EventKind = "drop"
	EventKindMergedLegs            EventKind = "mergedLegs"
	EventKindMergeConflict         EventKind = "mergeConflict"
	EventKindOverrule              EventKind = "overrule"
	EventKindRestartHeld           EventKind = "restartHeld"
	EventKindPause                 EventKind = "pause"
	EventKindPauseConfirmed        EventKind = "pauseConfirmed"
	EventKindPauseFailed           EventKind = "pauseFailed"
	EventKindResumePaused          EventKind = "resumePaused"
	EventKindContinuationsAdmitted EventKind = "continuationsAdmitted"
	EventKindContinuationLaunching EventKind = "continuationLaunching"
	EventKindContinuationLaunched  EventKind = "continuationLaunched"
	EventKindContinuationFailed    EventKind = "continuationFailed"
	EventKindContinuationAmbiguous EventKind = "continuationAmbiguous"
	EventKindRetryContinuation     EventKind = "retryContinuation"
	EventKindReconcileFailed       EventKind = "reconcileFailed"
)

// Event is something that happened to a run, or to one of the routes a run is
// working at once. Leg names the leg it is about; without it (nil) the event is
// about the run's focus, the first leg still going.
//
// Event holds one case of the event: Kind says which case it is, and
// the fields that case carries are set; the others are zero. Field names are
// shared between cases where they mean the same (Reason, Operation, Leg,
// Detail, Note). Optional payloads (String?) are pointers. Event is not
// encoded, so it has no JSON tags.
type Event struct {
	Kind EventKind

	// Session and SessionID are the agent session's name and its id; Folder is
	// the attempt folder (sessionStarted). Session alone is set by parkStopped.
	Session   string
	SessionID string
	Folder    string

	// Output is the file the session wrote (sessionFinished); nil when the
	// session failed or wrote nothing.
	Output *string
	// Detail says why (sessionFinished, watched, ticketWatched); nil when it
	// has no reason of its own.
	Detail *string
	// Note is the note an approval, a resume, or a leg's decision carries.
	Note *string
	// Outcome is the name of the approval outcome picked (decided).
	Outcome string
	// Sharing is the source choice made at a prompt stage (decided); empty takes
	// the stage's own.
	Sharing SourceSharing
	// Watch is the pull request event observed (watched).
	Watch WatchEvent
	// Ticket is the ticket event observed (ticketWatched).
	Ticket TicketEvent
	// Subject is the subject as the trigger saw it (subjectChanged).
	Subject Subject
	// Reason is why: the subject closed, the run is blocked, dropped, held or
	// a pause or continuation failed. The text is what the person reads.
	Reason string
	// Error is the failure text of a parkStopped event; nil when it worked.
	Error *string
	// Stage is the stage a resume goes to; nil means the current stage.
	Stage *string
	// Earlier is true when a resume replays the stage's latest closed session.
	// Earlier defaults to true, so every resume must be built with EventResume,
	// which sets it; a literal leaves it false.
	Earlier bool
	// Answer is what you answered a held agent stage with (answered); nil
	// means the session continues with what you said in the session itself.
	Answer *string
	// Legs are the leg branches Beltline merged (mergedLegs).
	Legs []string
	// Files are the files that conflicted (mergeConflict).
	Files []string
	// RestartStage is the agent stage an overrule restarts at (overrule). The
	// Named apart from Stage, which is optional.
	RestartStage string
	// Comments are the person's comments an overrule sends (overrule).
	Comments string
	// Decision is the AFK decision the overrule answers (overrule).
	Decision string
	// AttemptID is the attempt the decision ended (overrule).
	AttemptID string
	// Operation identifies a pause or continuation (pause, pauseConfirmed, …).
	Operation string
	// Conversation is the interrupted conversation (pauseConfirmed); nil when unknown.
	Conversation *string
	// Leg is the leg the event is about; nil is the run's focus.
	Leg *string
}

// EventResume is a resume event: the held or parked leg continues at stage (nil:
// the current stage) with note. Earlier is true by default.
func EventResume(stage, note, leg *string) Event {
	return Event{Kind: EventKindResume, Stage: stage, Note: note, Earlier: true, Leg: leg}
}

// EffectKind is which case of Effect a value is.
type EffectKind string

const (
	EffectKindComment     EffectKind = "comment"
	EffectKindNotify      EffectKind = "notify"
	EffectKindStopSession EffectKind = "stopSession"
	EffectKindStop        EffectKind = "stop"
	EffectKindStopForPark EffectKind = "stopForPark"
	EffectKindCleanUp     EffectKind = "cleanUp"
	EffectKindMergeLegs   EffectKind = "mergeLegs"
	EffectKindInterrupt   EffectKind = "interrupt"
	EffectKindCancelAfk   EffectKind = "cancelAfk"
)

// Effect is what the executor must do after an event.
// Kind says which case it is; the fields that case carries are set.
type Effect struct {
	Kind EffectKind

	// Text is a comment's body (comment).
	Text string
	// Title and Body are a notification's title and body (notify).
	Title string
	Body  string
	// Session is the agent session to stop (stop, stopForPark, interrupt).
	Session string
	// Legs are the leg branches to merge (mergeLegs).
	Legs []string
	// Operation and Leg name the generating session to interrupt (interrupt);
	// Leg is not optional here.
	Operation string
	Leg       string
}
