// The reference and the
// prompt are compared as text, against the phrases the tests pin.

package core

import (
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// assistantReferenceJSONBlocks are the bodies of the ```json blocks of the reference,
// in order, split at each ```json
// marker.
func assistantReferenceJSONBlocks(text string) []string {
	var blocks []string
	for _, part := range strings.Split(text, "```json\n")[1:] {
		blocks = append(blocks, strings.Split(part, "\n```")[0])
	}
	return blocks
}

// TestTheReferenceNamesEveryStageWatchActionAndTrigger
func TestTheReferenceNamesEveryStageWatchActionAndTrigger(t *testing.T) {
	t.Parallel()
	text := AssistantReferenceText()
	var names []string
	for _, stageType := range AllStageType {
		names = append(names, "`"+string(stageType)+"`")
	}
	for _, action := range AllThenAction {
		names = append(names, "`"+string(action)+"`")
	}
	for _, event := range AllWatchEvent {
		names = append(names, "`"+string(event)+"`")
	}
	for _, event := range AllTicketEvent {
		names = append(names, "`"+string(event)+"`")
	}
	for _, target := range AllWatchTarget {
		names = append(names, target.Title())
	}
	for _, kind := range AllTriggerKind {
		names = append(names, kind.Title())
	}
	for _, fallback := range AllFallback {
		names = append(names, "`"+string(fallback)+"`")
	}
	for _, workspace := range AllWorkspace {
		names = append(names, "`"+string(workspace)+"`")
	}
	for _, placeholder := range BriefFixedPlaceholders {
		names = append(names, "`{"+placeholder+"}`")
	}
	names = append(names, "`"+LibraryExamplesRepository+"`", "`library-repos/<owner>__<repo>/`")
	for _, name := range names {
		if !strings.Contains(text, name) {
			t.Errorf("missing %s", name)
		}
	}
}

// TestTheExamplesDecodeAndPassTheChecks
// The examples decode, and pass Save once the agents they pin are in the library.
func TestTheExamplesDecodeAndPassTheChecks(t *testing.T) {
	t.Parallel()
	blocks := assistantReferenceJSONBlocks(AssistantReferenceText())
	if len(blocks) != 2 {
		t.Fatalf("json blocks = %d, want 2", len(blocks))
	}
	var ticket Stage
	if err := json.Unmarshal([]byte(blocks[0]), &ticket); err != nil {
		t.Fatal(err)
	}
	if target := ticket.WatchTarget(); target == nil || *target != WatchTargetTicket {
		t.Fatalf("ticket example watches %v, want the ticket", target)
	}
	defaults := DefaultsTicketWatchOutcomes()
	var whens []string
	for _, outcome := range ticket.Outcomes {
		whens = append(whens, outcome.When)
	}
	var want []string
	for _, event := range AllTicketEvent {
		want = append(want, string(event))
	}
	index := func(when string) int {
		return slices.IndexFunc(defaults, func(outcome Outcome) bool { return outcome.When == when })
	}
	sort.SliceStable(want, func(a, b int) bool { return index(want[a]) < index(want[b]) })
	if !reflect.DeepEqual(whens, want) {
		t.Fatalf("ticket example outcomes = %v, want %v", whens, want)
	}
	var belt Belt
	if err := json.Unmarshal([]byte(blocks[1]), &belt); err != nil {
		t.Fatal(err)
	}
	hasWatch := slices.ContainsFunc(belt.Stages, func(stage Stage) bool {
		return stage.Kind() == StageKindWatch
	})
	if !hasWatch || len(belt.Forks) == 0 {
		t.Fatalf("belt example has watch %v and %d forks", hasWatch, len(belt.Forks))
	}
	files := map[string][]byte{"belts/issues/v1.json": []byte(blocks[1])}
	for _, agent := range SampleAgents() {
		files["agents/"+agent.ID+"/v"+strconv.Itoa(agent.Version)+".md"] = []byte(AgentFileWrite(agent))
	}
	if problems := LibraryCheckProblems(files, nil); len(problems) != 0 {
		t.Fatalf("problems = %v, want none", problems)
	}
}

// TestTheWholeLibraryIsCheckedApartFromWhatIsArchived
func TestTheWholeLibraryIsCheckedApartFromWhatIsArchived(t *testing.T) {
	t.Parallel()
	watch := NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Outcomes: []Outcome{
		{ID: "merged", Name: "merged", When: "merged", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})},
	}})
	belt := func(id string) []byte {
		data, err := jsonx.Marshal(NewBeltFromTrigger(BeltFromTriggerArgs{ID: id, Name: strings.ToUpper(id[:1]) + id[1:],
			Trigger: Trigger{Kind: TriggerKindManual}, Repos: []string{"acme/web"}, Stages: []Stage{watch}}))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	early := belt("early")
	files := map[string][]byte{
		"belts/early/v1.json":  early,
		"belts/old/v1.json":    belt("old"),
		"belts/old/state.json": []byte(`{"archived":true}`),
	}
	problems := LibraryCheckProblems(files, nil)
	if len(problems) == 0 {
		t.Fatal("no problems for the early belt")
	}
	for _, problem := range problems {
		if !strings.HasPrefix(problem, "belts/early/v1.json: ") {
			t.Fatalf("problem %q is not about the early belt", problem)
		}
	}
}

// TestThePromptListsProblemsAndHowToCheck
func TestThePromptListsProblemsAndHowToCheck(t *testing.T) {
	t.Parallel()
	check := "'/app/Beltline' --check-library '/data'"
	prompt := AssistantPromptPrompt(map[string][]byte{}, "Add a watch", []string{"belts/a/v1.json: Broken."}, &check)
	if !strings.Contains(prompt, "## Problems now\n- belts/a/v1.json: Broken.") {
		t.Fatalf("prompt lacks the problem list:\n%s", prompt)
	}
	if !strings.Contains(prompt, "run `'/app/Beltline' --check-library '/data'`") {
		t.Fatalf("prompt lacks the check command:\n%s", prompt)
	}
	if !strings.Contains(prompt, "### Watches") || !strings.HasSuffix(prompt, "Add a watch") {
		t.Fatalf("prompt lacks the watch section or does not end with the request")
	}
	plain := AssistantPromptPrompt(map[string][]byte{}, "Hi", nil, nil)
	if !strings.Contains(plain, "## Problems now\nNone.") || !strings.Contains(plain, "When the user passes one on") {
		t.Fatalf("plain prompt = %q", plain)
	}
}

// TestTheReferenceDescribesStageOwnedNamesAndOutput
func TestTheReferenceDescribesStageOwnedNamesAndOutput(t *testing.T) {
	t.Parallel()
	text := AssistantReferenceText()
	if !strings.Contains(text, "`{<placeholder>}`") {
		t.Error("no {<placeholder>} description")
	}
	if !strings.Contains(text, "Use `{output}` for the current stage's file.") {
		t.Error("no {output} description")
	}
	if strings.Contains(text, "for each agent key on the belt") {
		t.Error("describes agent keys on the belt")
	}
	if strings.Contains(text, "{<stage id>}") {
		t.Error("describes stage ids as placeholders")
	}
}
