// The problem types and the
// location constructors, then the save-time
// checks in engine_validation_checks.go and engine_validation_settings.go.

package core

import "strings"

// ProblemLocation is the belt part a problem belongs to. Kind is one of belt,
// fork, route, stage, trigger, repository or outcome; ID is that part's stable
// id; Field names the field when the problem is about one.
type ProblemLocation struct {
	Kind  string  `json:"kind"`
	ID    *string `json:"id,omitzero"`
	Field *string `json:"field,omitzero"`
}

// ProblemLocationStage is the location of a stage.
func ProblemLocationStage(id string) ProblemLocation {
	return ProblemLocation{Kind: "stage", ID: &id}
}

// ProblemLocationTrigger is the location of a trigger entry.
func ProblemLocationTrigger(id string) ProblemLocation {
	return ProblemLocation{Kind: "trigger", ID: &id}
}

// ProblemLocationRepository is the location of a repository on an entry.
func ProblemLocationRepository(entry, repo string) ProblemLocation {
	return ProblemLocation{Kind: "repository", ID: &entry, Field: &repo}
}

// ProblemLocationRoute is the location of a route.
func ProblemLocationRoute(id string) ProblemLocation {
	return ProblemLocation{Kind: "route", ID: &id}
}

// ProblemLocationFork is the location of a fork.
func ProblemLocationFork(id string) ProblemLocation {
	return ProblemLocation{Kind: "fork", ID: &id}
}

// LocatedProblem is one problem message with the part of the belt it is about, if any.
type LocatedProblem struct {
	Message  string
	Location *ProblemLocation
}

// ValidationFailure is a rejected request or belt: a message, the ordered
// problems, and where each problem is (nil when a problem has no field of its own).
type ValidationFailure struct {
	Message   string
	Problems  []string
	Locations []*ProblemLocation
}

// NewValidationFailure creates a failure with the message and problems; when
// there are no problems, the message is the one problem.
func NewValidationFailure(message string, problems []string) ValidationFailure {
	if len(problems) == 0 {
		problems = []string{message}
	}
	return ValidationFailure{Message: message, Problems: problems, Locations: make([]*ProblemLocation, len(problems))}
}

// ValidationFailureLocated creates a failure from located problems.
func ValidationFailureLocated(located []LocatedProblem) ValidationFailure {
	f := ValidationFailure{Problems: make([]string, len(located)), Locations: make([]*ProblemLocation, len(located))}
	messages := make([]string, len(located))
	for i, p := range located {
		messages[i] = p.Message
		f.Problems[i] = p.Message
		f.Locations[i] = p.Location
	}
	f.Message = strings.Join(messages, " ")
	return f
}

// Error returns the failure's message.
func (f ValidationFailure) Error() string {
	return f.Message
}
