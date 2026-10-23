// Golden checks of the run wire types: the run list and detail, the run command responses and the
// request bodies.

package api_test

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// TestRunRequestBodiesEncodeTheirFields checks that the request bodies the commands send carry
// their fields and leave out the optional ones that are nil. The requests have no golden: the
// server only decodes them.
func TestRunRequestBodiesEncodeTheirFields(t *testing.T) {
	t.Parallel()
	note := "later"
	answer := "yes"
	stage := "review"
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"decide", api.RunDecideRequest{Outcome: "approve"}, `{"outcome":"approve"}`},
		{"decide with note", api.RunDecideRequest{Outcome: "approve", Note: &note}, `{"outcome":"approve","note":"later"}`},
		{"answer acknowledged", api.RunAnswerRequest{}, `{}`},
		{"answer", api.RunAnswerRequest{Answer: &answer}, `{"answer":"yes"}`},
		{"resume", api.RunResumeRequest{}, `{}`},
		{"resume to a stage", api.RunResumeRequest{Stage: &stage}, `{"stage":"review"}`},
		{"leg", api.RunLegRequest{}, `{}`},
		{"attempt", api.RunAttemptRequest{Attempt: "plan-epic-1"}, `{"attempt":"plan-epic-1"}`},
		{"afk", api.RunAfkRequest{Mode: "always"}, `{"mode":"always"}`},
		{"empty", api.RunEmptyRequest{}, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			same(t, tc.name, tc.value, []byte(tc.want))
		})
	}
}

// TestRunTaskRequestEncodesItsFields checks the request a task on a manual belt sends: its
// entry is optional and leaves out when nil.
func TestRunTaskRequestEncodesItsFields(t *testing.T) {
	t.Parallel()
	task := api.RunTaskRequest{Belt: "tasks", Repo: "acme/web", Title: "Fix the thing", Body: "Details"}
	same(t, "task", task, []byte(`{"belt":"tasks","repo":"acme/web","title":"Fix the thing","body":"Details"}`))
}
