// Contract tests of the run reads that the runs routes serve.

package servertest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TestAgentStagesCarryTheirAdjustHelpAndApprovalsDoNot
func TestAgentStagesCarryTheirAdjustHelpAndApprovalsDoNot(t *testing.T) {
	t.Parallel()
	f := ReadOnlyFixture(t)
	run := f.Coordinator.ReadRun(f.ActionRunID)
	if run == nil {
		t.Fatal("the action run is gone")
	}
	for _, stage := range run.Stages {
		// The page asks for the stage it shows, and that stage's help is the one it gets.
		focus := stage.ID
		snapshot := f.Coordinator.ReadRunViewSnapshot(run.ID, &focus)
		if snapshot == nil {
			t.Fatal("no run view snapshot")
		}
		response := api.NewRunViewResponse(*snapshot)
		var wire *api.RunStageWire
		for i := range response.Stages {
			if response.Stages[i].ID == stage.ID {
				wire = &response.Stages[i]
			}
		}
		if wire == nil {
			t.Fatalf("stage %s is not in the view", stage.ID)
		}
		if stage.Kind() != core.StageKindAgent {
			if wire.Adjust != nil {
				t.Fatalf("%s has no brief to adjust", stage.ID)
			}
			continue
		}
		if wire.Adjust == nil {
			t.Fatalf("%s needs its Adjust stage help", stage.ID)
		}
		key := func(s core.Stage) *string { return f.Coordinator.KeyIn(s, *run) }
		route := func(s core.Stage) string { return core.StageKeysPathOf(s, run.Layout()) }
		expected := core.PlaceholderHelpStages(run.Stages, new(stage.ID), key, route, !run.UsesStageKeys())
		if !reflect.DeepEqual(wire.Adjust.Placeholders, expected) {
			t.Fatalf("%s: placeholders differ from the shared help", stage.ID)
		}
		wantNote := core.StageAdjustResolutionNote
		if !run.UsesStageKeys() {
			wantNote = core.StageAdjustLegacyResolutionNote
		}
		if wire.Adjust.Note != wantNote {
			t.Fatalf("%s: resolution note %q", stage.ID, wire.Adjust.Note)
		}
		var brief *string
		if agent := f.Coordinator.AgentFor(stage.Agent, *run); agent != nil {
			brief = new(agent.Brief)
		}
		if !reflect.DeepEqual(wire.Adjust.AgentBrief, brief) {
			t.Fatalf("%s: agent brief %v, want %v", stage.ID, wire.Adjust.AgentBrief, brief)
		}
	}
}

// TestAttemptOutputParsesLongMarkdownForTheNamedAttemptOnly
func TestAttemptOutputParsesLongMarkdownForTheNamedAttemptOnly(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	f.SeedViews()
	run := f.Coordinator.ReadRun(f.ActionRunID)
	if run == nil {
		t.Fatal("the action run is gone")
	}
	var attempt core.Attempt
	found := false
	for _, candidate := range run.Attempts {
		if candidate.ID == f.ActionAttemptID {
			attempt, found = candidate, true
		}
	}
	if !found {
		t.Fatal("the action attempt is gone")
	}
	var stage core.Stage
	for _, candidate := range run.Stages {
		if candidate.ID == attempt.StageID {
			stage = candidate
		}
	}
	text := strings.Join([]string{
		"# Release notes",
		"",
		"A paragraph with enough words to show that short lines reflow around the output column.",
		"This sentence is deliberately kept on its own line because the next word would not fit.",
		"hard line one  ",
		"hard line two",
		"",
		"- first item",
		"- second item with a wrapped",
		"  continuation that remains part of the same item",
		"",
		"1. numbered item",
		"",
		"```swift",
		`let greeting = "hello  "`,
		"print(greeting)",
		"```",
		"",
		"```",
		"unclosed code keeps trailing spaces  ",
		"Outcome: complete",
	}, "\n")
	if err := os.WriteFile(filepath.Join(f.Coordinator.Paths().AttemptFolder(*run, attempt), stage.Writes()), []byte(text), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	path := casePath("runAttemptOutput", run.ID, attempt.ID)
	reply := getCase(f, path)
	if reply.Status != http.StatusOK {
		t.Fatalf("output: HTTP %d", reply.Status)
	}
	var response struct {
		Availability string          `json:"availability"`
		Blocks       json.RawMessage `json:"blocks"`
	}
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("output: %v", err)
	}
	parsed, err := jsonx.Marshal(core.MarkdownBlockParse(core.DocumentHoisting(core.OutcomeParserStrip(text), core.DocumentHoistingDefaultSection)))
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Blocks) != string(parsed) {
		t.Fatal("the blocks are not the shared Markdown parse of the output")
	}
	if response.Availability != "available" {
		t.Fatalf("availability %q, want available", response.Availability)
	}
	var blocks []struct {
		Type  string `json:"type"`
		Level int    `json:"level"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(response.Blocks, &blocks); err != nil {
		t.Fatalf("blocks: %v", err)
	}
	hasTitle, hasCode := false, false
	for _, block := range blocks {
		if block.Type == "heading" && block.Level == 1 && block.Text == "Release notes" {
			hasTitle = true
		}
		if strings.Contains(block.Text, "hello  ") {
			hasCode = true
		}
	}
	if !hasTitle || !hasCode {
		t.Fatalf("title %v, code %v: want the heading and the code block", hasTitle, hasCode)
	}
	if strings.Contains(reply.Text(), "Outcome: complete") {
		t.Fatal("the outcome footer reached the output")
	}
	CheckGolden(t, "long attempt Markdown", "run-attempt-output-long.json", reply, f.Root)
}

// TestAttemptOutputDistinguishesMissingOpenCompletedAndNonAgentAttempts
func TestAttemptOutputDistinguishesMissingOpenCompletedAndNonAgentAttempts(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	agent := core.NewStage(core.StageArgs{ID: "agent step", Type: core.StageTypeAgent, Name: new("Agent"), Writes: new("report.md"), Outcomes: []core.Outcome{}})
	gate := core.NewStage(core.StageArgs{ID: "approval", Type: core.StageTypeApprove, Name: new("Approval"), Outcomes: []core.Outcome{}})
	belt := core.Belt{
		ID: "output-tests", Name: "Output tests", Version: 1, Policies: core.DefaultPolicies(),
		Triggers: []core.TriggerEntry{{ID: "1", Trigger: core.Trigger{Kind: core.TriggerKindManual}, Repos: []string{"acme/read"}}},
		Stages:   []core.Stage{agent, gate},
	}
	subject := core.Subject{Kind: core.SubjectKindManual, Repo: "acme/read", Key: "missing/output", Title: "Output cases"}
	run := core.NewRun(core.RunArgs{Belt: belt, Subject: subject, Now: ContractNow})
	now := jsonxDate(ContractNow)
	live := core.NewAttempt(core.AttemptArgs{StageID: agent.ID, Number: 1, StartedAt: now, Folder: "open"})
	live.Session = new("live-session")
	complete := core.NewAttempt(core.AttemptArgs{StageID: agent.ID, Number: 2, StartedAt: now, Folder: "complete"})
	complete.EndedAt = &now
	decision := core.NewAttempt(core.AttemptArgs{StageID: gate.ID, Number: 1, StartedAt: now, Note: new("Approve"), Folder: "decision"})
	run.Attempts = []core.Attempt{live, complete, decision}
	f.Coordinator.AppendRunForContract(run)

	base := "/api/v1/runs/" + contractSegment(run.ID) + "/attempts/"
	outputPath := func(attempt core.Attempt) string {
		return base + contractSegment(attempt.ID) + "/output"
	}

	openReply := getCase(f, outputPath(live))
	var openResponse api.AttemptOutputResponse
	if err := routes.JSONDecode(openReply.Body, &openResponse); err != nil || openReply.Status != http.StatusOK {
		t.Fatalf("open attempt: HTTP %d %v", openReply.Status, err)
	}
	if openResponse.Availability != "missing" || len(openResponse.Blocks) != 0 {
		t.Fatalf("open attempt: availability %q, %d blocks", openResponse.Availability, len(openResponse.Blocks))
	}
	if openResponse.Message == nil || !strings.Contains(*openResponse.Message, "working") {
		t.Fatalf("open attempt message %v, want the working note", openResponse.Message)
	}

	completeReply := getCase(f, outputPath(complete))
	var completeResponse api.AttemptOutputResponse
	if err := routes.JSONDecode(completeReply.Body, &completeResponse); err != nil || completeReply.Status != http.StatusOK {
		t.Fatalf("completed attempt: HTTP %d %v", completeReply.Status, err)
	}
	if completeResponse.Availability != "missing" || len(completeResponse.Blocks) != 0 {
		t.Fatalf("completed attempt: availability %q, %d blocks", completeResponse.Availability, len(completeResponse.Blocks))
	}
	if completeResponse.Message == nil || !strings.Contains(*completeResponse.Message, "wrote no report.md") {
		t.Fatalf("completed attempt message %v, want the no-output note", completeResponse.Message)
	}

	gateReply := getCase(f, outputPath(decision))
	var gateResponse api.AttemptOutputResponse
	if err := routes.JSONDecode(gateReply.Body, &gateResponse); err != nil || gateReply.Status != http.StatusOK {
		t.Fatalf("gate attempt: HTTP %d %v", gateReply.Status, err)
	}
	if gateResponse.Availability != "notApplicable" || len(gateResponse.Blocks) != 0 || gateResponse.Message != nil {
		t.Fatalf("gate attempt: %+v, want notApplicable with no message", gateResponse)
	}

	unknown := getCase(f, base+"unknown/output")
	if unknown.Status != http.StatusNotFound {
		t.Fatalf("unknown attempt: HTTP %d, want 404", unknown.Status)
	}
	CheckBody(t, "unknown attempt", "error-not-found.json", unknown.Body, f.Root)

	foreign := getCase(f, "/api/v1/runs/"+contractSegment(f.ActionRunID)+"/attempts/"+contractSegment(live.ID)+"/output")
	if foreign.Status != http.StatusNotFound {
		t.Fatalf("a foreign attempt: HTTP %d, want 404", foreign.Status)
	}
	CheckBody(t, "foreign attempt", "error-not-found.json", foreign.Body, f.Root)

	for _, attempt := range []core.Attempt{live, decision} {
		artifactPath := base + contractSegment(attempt.ID) + "/artifacts"
		empty := getCase(f, artifactPath)
		var response api.AttemptArtifactsResponse
		if err := routes.JSONDecode(empty.Body, &response); err != nil || empty.Status != http.StatusOK {
			t.Fatalf("artifacts: HTTP %d %v", empty.Status, err)
		}
		if len(response.Artifacts) != 0 {
			t.Fatalf("artifacts of %s: %d, want none", attempt.ID, len(response.Artifacts))
		}
	}
}

// TestArtifactListFiltersSortsSignsEncodesAndDeliversConfinedFiles
func TestArtifactListFiltersSortsSignsEncodesAndDeliversConfinedFiles(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	run := f.Coordinator.ReadRun(f.ActionRunID)
	if run == nil {
		t.Fatal("the action run is gone")
	}
	var attempt core.Attempt
	for _, candidate := range run.Attempts {
		if candidate.ID == f.ActionAttemptID {
			attempt = candidate
		}
	}
	folder := f.Coordinator.Paths().ArtifactsFolder(*run, attempt)
	examples := map[string][]byte{
		"A file.html":          []byte("space"),
		"Zeta.HTML":            []byte("mixed-case"),
		"E\u0301te\u0301.html": []byte("unicode"),
	}
	for name, data := range examples {
		if err := os.WriteFile(filepath.Join(folder, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(folder, "notes.txt"), []byte("not html"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "folder.html"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	outside := filepath.Join(f.Root, "outside.html")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(folder, "link.html")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	path := "/api/v1/runs/" + contractSegment(run.ID) + "/attempts/" + contractSegment(attempt.ID) + "/artifacts"
	reply := getCase(f, path)
	if reply.Status != http.StatusOK {
		t.Fatalf("artifacts: HTTP %d", reply.Status)
	}
	var response api.AttemptArtifactsResponse
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("artifacts: %v", err)
	}
	if response.RunID != run.ID || response.AttemptID != attempt.ID {
		t.Fatalf("ids %s %s, want %s %s", response.RunID, response.AttemptID, run.ID, attempt.ID)
	}
	var names []string
	for _, artifact := range response.Artifacts {
		names = append(names, artifact.Name)
	}
	if want := []string{"A file.html", f.ArtifactName(), "Zeta.HTML", "E\u0301te\u0301.html"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("artifact names %v, want %v", names, want)
	}
	wantExpiry := ContractNow.Add(60 * time.Second)
	for _, artifact := range response.Artifacts {
		if !artifact.ExpiresAt.Equal(wantExpiry) {
			t.Fatalf("%s expires %v, want %v", artifact.Name, artifact.ExpiresAt.Time, wantExpiry)
		}
		if !strings.Contains(artifact.URL, contractSegment(artifact.Name)) || strings.Contains(artifact.URL, "beltline_auth") {
			t.Fatalf("url %q for %s", artifact.URL, artifact.Name)
		}
		expected, ok := examples[artifact.Name]
		if !ok {
			expected = []byte(ContractArtifactHTML)
		}
		if artifact.SizeBytes != int64(len(expected)) {
			t.Fatalf("%s size %d, want %d", artifact.Name, artifact.SizeBytes, len(expected))
		}
		delivered := f.Do(Request{Method: "GET", Path: artifact.URL})
		if delivered.Status != http.StatusOK || string(delivered.Body) != string(expected) {
			t.Fatalf("%s: HTTP %d, body %q", artifact.Name, delivered.Status, delivered.Body)
		}
		if got := delivered.Header.Get("Content-Security-Policy"); got != "sandbox allow-scripts" {
			t.Fatalf("%s: CSP %q", artifact.Name, got)
		}
		if got := delivered.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s: nosniff %q", artifact.Name, got)
		}
		if delivered.Header.Get("Set-Cookie") != "" {
			t.Fatalf("%s: a cookie was set", artifact.Name)
		}
	}

	f.Clock.Set(ContractNow.Add(60 * time.Second))
	if expired := f.Do(Request{Method: "GET", Path: response.Artifacts[0].URL}); expired.Status != http.StatusUnauthorized {
		t.Fatalf("an expired link: HTTP %d, want 401", expired.Status)
	}
	f.Clock.Set(ContractNow)
	if err := os.Remove(filepath.Join(folder, "A file.html")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if gone := f.Do(Request{Method: "GET", Path: response.Artifacts[0].URL}); gone.Status != http.StatusNotFound {
		t.Fatalf("a removed file: HTTP %d, want 404", gone.Status)
	}
}
