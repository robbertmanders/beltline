// Tests of the Pi, Copilot, Claude, Codex and Cursor usage parsing, and the cost presentation of
// attempts. Tests that need the coordinator, the demo or the run's JSON encoding are not in this file.

package runner

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// piTurn is a Pi assistant line with a usage report of one call.
const piTurn = `{"type":"message","message":{"role":"assistant","usage":{"input":401,"output":93,"cacheRead":50560,"cacheWrite":0,"reasoning":15,"totalTokens":51054,"cost":{"total":0.006788}}}}`

// usageOf is a usage with a dollar amount only.
func usageOf(nanos int) core.Usage {
	return core.NewUsage(core.UsageArgs{CostNanos: new(nanos)})
}

// checkInt fails when an optional count is not want.
func checkInt(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", name, got, want)
	}
}

// claudeTurn is one Claude assistant line with a usage block of the given counts.
func claudeTurn(id, model string, input int, extra string) string {
	return `{"type":"assistant","uuid":"` + id + `-` + strconv.Itoa(input) + `","sessionId":"s","message":{"id":"` + id + `","role":"assistant","model":"` + model +
		`","usage":{"input_tokens":` + strconv.Itoa(input) + `,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0` + extra + `}}}`
}

// attemptEnded is an attempt of stage that ran for a minute up to ended, with the given session and usage.
func attemptEnded(stage string, ended time.Time, session *string, usage *core.Usage) core.Attempt {
	attempt := core.NewAttempt(core.AttemptArgs{StageID: stage, Number: 1, StartedAt: jsonx.Date{Time: ended.Add(-time.Minute)}, Folder: stage})
	endedAt := jsonx.Date{Time: ended}
	attempt.EndedAt = &endedAt
	attempt.Session = session
	attempt.Usage = usage
	return attempt
}

// usageRun is a run of the default belt for a test subject on a/b #1.
func usageRun(t testing.TB, now time.Time) core.Run {
	t.Helper()
	belt := core.SampleBelt(nil, "agent", core.SampleAgents(), true)
	return core.NewRun(core.RunArgs{Belt: belt, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "a/b", Number: 1, Title: "Test"}, Now: now})
}

func TestPiCountsOnlyAssistantMessagesAfterMark(t *testing.T) {
	t.Parallel()
	noUsage := `{"type":"message","message":{"role":"assistant"}}`
	user := `{"type":"message","message":{"role":"user"}}`
	log := strings.Join([]string{piTurn, user, noUsage, "invalid", piTurn}, "\n")
	expected := core.NewUsage(core.UsageArgs{CostNanos: new(6_788_000), Input: new(401), Output: new(93), CacheRead: new(50560), CacheWrite: new(0), Reasoning: new(15), TotalTokens: new(51054)})
	sum := core.UsageAdd(expected, expected)
	mustEqual(t, &sum, PiUsageOf(log, 0))
	mustEqual(t, &expected, PiUsageOf(log, 2))
	if PiUsageOf(log, 3) != nil {
		t.Fatalf("usage after every message = %+v", PiUsageOf(log, 3))
	}
	if PiUsageOf(noUsage, 0) != nil || PiUsageOf(user, 0) != nil || PiUsageOf("", 0) != nil {
		t.Fatalf("usage without a reported cost")
	}
	if got := (PiAdapter{}).Usage(filepath.Join(t.TempDir(), "missing"), "missing-session", "", 0, 0); got != nil {
		t.Fatalf("usage of a missing session = %+v", got)
	}
	zero := strings.Replace(piTurn, "0.006788", "0", 1)
	if got := PiUsageOf(zero, 0); got == nil || got.CostNanos == nil || *got.CostNanos != 0 {
		t.Fatalf("zero-cost usage = %+v", got)
	}
}

func TestPiToolCallsCountTowardsTheMarkAndUsage(t *testing.T) {
	t.Parallel()
	tool := strings.Replace(piTurn, `"role":"assistant"`, `"role":"assistant","stopReason":"toolUse","content":[{"type":"toolCall","id":"tool-1","name":"bash"}]`, 1)
	log := tool + "\n" + piTurn
	if got := PiUsageOf(log, 0); got == nil || got.TotalTokens == nil || *got.TotalTokens != 102108 {
		t.Fatalf("total = %+v", got)
	}
	mustEqual(t, PiUsageOf(piTurn, 0), PiUsageOf(log, 1))
	if PiUsageOf(log, 2) != nil {
		t.Fatalf("usage after both messages")
	}
}

func TestCopilotUsageIsTheLastCumulativeReportAfterTheMark(t *testing.T) {
	t.Parallel()
	first := `{"type":"session.task_complete","data":{"total_nano_aiu":11000000000}}`
	last := `{"type":"session.task_complete","data":{"total_nano_aiu":34000000000}}`
	log := first + "\n" + last
	if got := CopilotNanoAIU(log); got == nil || *got != 34_000_000_000 {
		t.Fatalf("nanoAIU = %v", got)
	}
	if got := CopilotUsageOf(log, 0); got == nil || *got.CostNanos != 340_000_000 {
		t.Fatalf("usage = %+v", got)
	}
	if got := CopilotUsageOf(log, 11_000_000_000); got == nil || *got.CostNanos != 230_000_000 {
		t.Fatalf("usage after the mark = %+v", got)
	}
	if got := CopilotUsageOf(log, 40_000_000_000); got == nil || *got.CostNanos != 0 {
		t.Fatalf("usage past the total = %+v", got)
	}
	if CopilotUsageOf("{}", 0) != nil {
		t.Fatalf("usage of a log without a total")
	}
	if got := CopilotUsageOf(`{"type":"session.task_complete","data":{"total_nano_aiu":0}}`, 0); got == nil || *got.CostNanos != 0 {
		t.Fatalf("zero usage = %+v", got)
	}
}

func TestCopilotCheckpointsAndShutdownsReadTheCurrentTotal(t *testing.T) {
	t.Parallel()
	checkpoint := `{"type":"session.usage_checkpoint","data":{"totalNanoAiu":11000000000,"totalPremiumRequests":1}}`
	completion := `{"type":"session.task_complete","data":{"summary":"Done","success":true}}`
	shutdown := `{"type":"session.shutdown","data":{"totalNanoAiu":34000000000,"totalPremiumRequests":2}}`
	log := strings.Join([]string{checkpoint, completion, shutdown, shutdown}, "\n")
	if got := CopilotNanoAIU(checkpoint); got == nil || *got != 11_000_000_000 {
		t.Fatalf("checkpoint nanoAIU = %v", got)
	}
	if got := CopilotNanoAIU(log); got == nil || *got != 34_000_000_000 {
		t.Fatalf("nanoAIU = %v", got)
	}
	if got := CopilotUsageOf(log, 11_000_000_000); got == nil || *got.CostNanos != 230_000_000 {
		t.Fatalf("usage = %+v", got)
	}
	if CopilotUsageOf(completion, 0) != nil {
		t.Fatalf("usage of a completion without a total")
	}
	if got := CopilotUsageOf(`{"type":"session.shutdown","data":{"totalNanoAiu":0}}`, 0); got == nil || *got.CostNanos != 0 {
		t.Fatalf("zero shutdown = %+v", got)
	}
	if CopilotUsageOf(`{"type":"session.usage_checkpoint","data":{"totalNanoAiu":-1}}`, 0) != nil {
		t.Fatalf("negative total accepted")
	}
	// The attempt's cost, as the run presents it, is the same figure.
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	run := usageRun(t, now)
	usage := CopilotUsageOf(log, 11_000_000_000)
	run.Attempts = []core.Attempt{attemptEnded("build", now, nil, usage)}
	if got := core.CostAmount(core.CostToday([]core.Run{run}, time.UTC, now)); got != "$0.23" {
		t.Fatalf("today = %q, want $0.23", got)
	}
}

func TestCopilotFoldersMatchOnlyByExactNameOrWorkspaceName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	log := `{"type":"session.shutdown","data":{"totalNanoAiu":34000000000}}`

	// A session folder named by the first 100 characters of the session name.
	name := strings.Repeat("session-", 20)
	testkit.WriteFile(t, filepath.Join(root, core.PrefixCharacters(name, 100)+"-suffix", "events.jsonl"), log)
	adapter := CopilotAdapter{StateRoot: root}
	for _, prefix := range []string{name, "missing", ""} {
		if got := adapter.events(prefix, nil); got != nil {
			t.Fatalf("prefix or missing name %q matched: %q", prefix, *got)
		}
	}
	testkit.WriteFile(t, filepath.Join(root, core.PrefixCharacters(name, 100), "events.jsonl"), "exact")
	if got := adapter.events(name, nil); got == nil || *got != "exact" {
		t.Fatalf("exact folder = %v", got)
	}

	// UUID folders are found by the workspace.yaml name, quoted or not, but an ambiguous name matches nothing.
	wsName := strings.Repeat("bl-session-", 12)
	key := core.PrefixCharacters(wsName, 100)
	uuid := filepath.Join(root, "uuid-folder-1")
	testkit.WriteFile(t, filepath.Join(uuid, "events.jsonl"), log)
	for _, value := range []string{key, `"` + key + `"`, "'" + key + "'"} {
		testkit.WriteFile(t, filepath.Join(uuid, "workspace.yaml"), "id: uuid-folder-1\nname: "+value+"\nuser_named: true\n")
		if got := adapter.events(wsName, nil); got == nil || *got != log {
			t.Fatalf("events for name %s = %v", value, got)
		}
	}
	if got := adapter.events(key[:len(key)-1], nil); got != nil {
		t.Fatalf("prefix matched")
	}
	testkit.WriteFile(t, filepath.Join(root, "uuid-folder-2", "workspace.yaml"), "name: "+key+"\n")
	if got := adapter.events(wsName, nil); got != nil {
		t.Fatalf("ambiguous name matched")
	}
}

func TestClaudeCountsToolUseCacheAndSidechainOnce(t *testing.T) {
	t.Parallel()
	usage := ClaudeUsageUsage(fixtureText(t, "claude-transcript.jsonl"), new(0), "sess-1", nil)
	if usage == nil {
		t.Fatal("no usage")
	}
	if *usage.CostNanos != 11_604_000 || usage.Provenance == nil || *usage.Provenance != core.CostProvenanceEstimated {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Coverage != core.UsageCoverageComplete {
		t.Errorf("coverage = %v", usage.Coverage)
	}
	checkInt(t, "input", usage.Input, 1018)
	checkInt(t, "output", usage.Output, 209)
	checkInt(t, "cacheRead", usage.CacheRead, 9800)
	checkInt(t, "cacheWrite", usage.CacheWrite, 600)
	if usage.Reasoning != nil {
		t.Errorf("reasoning = %d, want nil", *usage.Reasoning)
	}
	checkInt(t, "total", usage.TotalTokens, 11627)
	if usage.Rate == nil || usage.Rate.Revision != RateTableClaude().Checked || len(usage.Rate.Models) != 1 || usage.Rate.Models[0] != "claude-sonnet-4-6" {
		t.Errorf("rate = %+v", usage.Rate)
	}
	if ClaudeUsageUsage(fixtureText(t, "claude-transcript.jsonl"), nil, "sess-1", nil) != nil {
		t.Errorf("usage without a baseline")
	}
	after := ClaudeUsageUsage(fixtureText(t, "claude-transcript.jsonl"), new(2), "sess-1", nil)
	checkInt(t, "total after 2 lines", after.TotalTokens, 11627-5800)
}

func TestClaudeCountsOneResponseSplitAcrossLinesOnce(t *testing.T) {
	t.Parallel()
	usage := `{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`
	var lines []string
	for i, block := range []string{"thinking", "text", "tool_use"} {
		lines = append(lines, `{"type":"assistant","uuid":"u`+strconv.Itoa(i)+`","sessionId":"s","message":{"id":"msg-1","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"`+block+`"}],"usage":`+usage+`}}`)
	}
	counted := ClaudeUsageUsage(strings.Join(lines, "\n"), new(0), "s", nil)
	if counted == nil {
		t.Fatal("no usage")
	}
	checkInt(t, "input", counted.Input, 100)
	checkInt(t, "output", counted.Output, 20)
	checkInt(t, "total", counted.TotalTokens, 120)
}

func TestClaudeKeepsKnownSubtotalsWhenRecordsAreUnusable(t *testing.T) {
	t.Parallel()
	usage := ClaudeUsageUsage(fixtureText(t, "claude-gaps.jsonl"), new(0), "sess-1", nil)
	if usage == nil || *usage.CostNanos != 500_018_000 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Coverage != core.UsageCoveragePartial || *usage.Provenance != core.CostProvenanceEstimated {
		t.Errorf("coverage/provenance = %v %v", usage.Coverage, usage.Provenance)
	}
	checkInt(t, "input", usage.Input, 4)
	checkInt(t, "output", usage.Output, 4)
	checkInt(t, "cacheRead", usage.CacheRead, 0)
	checkInt(t, "cacheWrite", usage.CacheWrite, 0)
	checkInt(t, "total", usage.TotalTokens, 6)
	zero := `{"type":"assistant","uuid":"z","sessionId":"sess-1","costUSD":0,"message":{"id":"z","role":"assistant","model":"nope","usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`
	got := ClaudeUsageUsage(zero, new(0), "sess-1", nil)
	if got == nil || got.CostNanos == nil || *got.CostNanos != 0 || got.Provenance == nil || *got.Provenance != core.CostProvenanceReported {
		t.Errorf("zero reported cost = %+v", got)
	}
}

func TestClaudeKeepsWhatIsKnownWhenACounterIsAbsentOrInvalid(t *testing.T) {
	t.Parallel()
	usage := ClaudeUsageUsage(fixtureText(t, "claude-absent-cache.jsonl"), new(0), "sess-1", nil)
	if usage == nil {
		t.Fatal("no usage")
	}
	checkInt(t, "input", usage.Input, 7)
	checkInt(t, "output", usage.Output, 3)
	checkInt(t, "cacheRead", usage.CacheRead, 11)
	if usage.CacheWrite != nil || usage.TotalTokens != nil || usage.CostNanos != nil || usage.Provenance != nil {
		t.Errorf("unexpected fields: %+v", usage)
	}
	if usage.Coverage != core.UsageCoveragePartial {
		t.Errorf("coverage = %v", usage.Coverage)
	}
	badReasoning := `{"type":"assistant","uuid":"r","sessionId":"sess-1","message":{"id":"r","role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"reasoning_tokens":-1}}}`
	kept := ClaudeUsageUsage(badReasoning, new(0), "sess-1", nil)
	if kept == nil || *kept.Input != 1 || *kept.Output != 1 || *kept.CacheRead != 0 || *kept.CacheWrite != 0 || kept.Reasoning != nil {
		t.Fatalf("kept = %+v", kept)
	}
	checkInt(t, "total", kept.TotalTokens, 2)
	checkInt(t, "cost", kept.CostNanos, 18_000)
	if kept.Coverage != core.UsageCoveragePartial || *kept.Provenance != core.CostProvenanceEstimated {
		t.Errorf("coverage/provenance = %v %v", kept.Coverage, kept.Provenance)
	}
	reported := `{"type":"assistant","uuid":"p","sessionId":"sess-1","costUSD":1.25,"message":{"id":"p","role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":-1,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`
	priced := ClaudeUsageUsage(reported, new(0), "sess-1", nil)
	if priced == nil || *priced.CostNanos != 1_250_000_000 || *priced.Provenance != core.CostProvenanceReported {
		t.Fatalf("priced = %+v", priced)
	}
	checkInt(t, "output", priced.Output, 4)
	if priced.Input != nil || priced.TotalTokens != nil || priced.Coverage != core.UsageCoveragePartial {
		t.Errorf("priced fields = %+v", priced)
	}
	split := `{"type":"assistant","uuid":"w","sessionId":"sess-1","message":{"id":"w","role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":2,"output_tokens":2,"cache_read_input_tokens":2,"cache_creation_input_tokens":5}}}`
	unsplit := ClaudeUsageUsage(split, new(0), "sess-1", nil)
	if unsplit == nil || *unsplit.Input != 2 || *unsplit.Output != 2 || *unsplit.CacheRead != 2 || *unsplit.CacheWrite != 5 {
		t.Fatalf("unsplit = %+v", unsplit)
	}
	checkInt(t, "total", unsplit.TotalTokens, 11)
	if unsplit.CostNanos != nil || unsplit.Coverage != core.UsageCoverageUntracked {
		t.Errorf("unsplit cost = %+v", unsplit)
	}
}

func TestClaudePricesEachModelAndLeavesUnknownModelsUntracked(t *testing.T) {
	t.Parallel()
	rates := NewRateTable(RateTableArgs{Provider: "test", Source: "https://example.test/claude", Checked: "test-rev", Models: map[string]ModelRate{
		"model-a": {Standard: Prices{Input: rat(t, "3"), Output: rat(t, "15"), CacheRead: rat(t, "0.30"), CacheWrite5m: rat(t, "3.75"), CacheWrite1h: rat(t, "6")}},
		"model-b": {Standard: Prices{Input: rat(t, "1"), Output: rat(t, "5"), CacheRead: rat(t, "0.10"), CacheWrite5m: rat(t, "1.25"), CacheWrite1h: rat(t, "2")}},
	}})
	mixed := strings.Join([]string{claudeTurn("model-a", "model-a", 2, ""), claudeTurn("model-b", "model-b", 4, "")}, "\n")
	usage := ClaudeUsageUsage(mixed, new(0), "s", &rates)
	// 2*3*1000 + 1*15*1000 + 4*1*1000 + 1*5*1000
	if usage == nil || *usage.CostNanos != 6_000+15_000+4_000+5_000 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Rate == nil || usage.Rate.Revision != "test-rev" || len(usage.Rate.Models) != 2 || usage.Rate.Models[0] != "model-a" || usage.Rate.Models[1] != "model-b" {
		t.Errorf("rate = %+v", usage.Rate)
	}
	alias := ClaudeUsageUsage(claudeTurn("claude-sonnet-4-5", "claude-sonnet-4-5", 10, ""), new(0), "s", &rates)
	if alias == nil || alias.CostNanos != nil || *alias.Input != 10 || alias.Coverage != core.UsageCoverageUntracked {
		t.Errorf("alias = %+v", alias)
	}
	batch := ClaudeUsageUsage(claudeTurn("model-a", "model-a", 1, `,"service_tier":"batch"`), new(0), "s", &rates)
	if batch == nil || batch.CostNanos != nil {
		t.Errorf("batch = %+v", batch)
	}
	checkInt(t, "batch total", batch.TotalTokens, 2)
	if half := RateTableNanos(1, rat(t, "0.0005")); half == nil || *half != 1 {
		t.Errorf("half = %v", half)
	}
	if got := RateTableNanos(2, rat(t, "0.0005")); got == nil || *got != 1 {
		t.Errorf("2 tokens = %v", got)
	}
	if RateTableNanos(int(^uint(0)>>1), rat(t, "3")) != nil {
		t.Errorf("overflowing product not refused")
	}
	data, err := jsonx.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded core.Usage
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.CostNanos == nil || *reloaded.CostNanos != *usage.CostNanos || reloaded.Rate == nil || reloaded.Rate.Revision != "test-rev" {
		t.Errorf("reloaded = %+v", reloaded)
	}
}

func TestClaudeOpus55EstimatesAtListPriceAndLeavesSuffixesUntracked(t *testing.T) {
	t.Parallel()
	base := RateTableClaude().Rate("claude-opus-5-5")
	if base == nil {
		t.Fatal("claude-opus-5-5 is not priced")
	}
	window := RateTableClaude().Rate("claude-opus-5-5[1m]")
	if window == nil || !pricesEqual(&base.Standard, &window.Standard) {
		t.Fatalf("[1m] rate differs from the base rate")
	}
	if !ratEqual(base.Standard.Input, rat(t, "4")) || !ratEqual(base.Standard.Output, rat(t, "20")) ||
		!ratEqual(base.Standard.CacheWrite5m, rat(t, "5")) || !ratEqual(base.Standard.CacheWrite1h, rat(t, "8")) || !ratEqual(base.Standard.CacheRead, rat(t, "0.20")) {
		t.Errorf("list price = %+v", base.Standard)
	}
	opusTurn := func(model, cost string) string {
		return `{"type":"assistant","uuid":"u","sessionId":"s"` + cost + `,"message":{"id":"m","role":"assistant","model":"` + model +
			`","usage":{"input_tokens":1000,"output_tokens":100,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":100}}}}`
	}
	// 1000×$4 + 100×$20 + 1000×$0.20 + 100×$5 + 100×$8, in nano-dollars.
	const expected = 4_000_000 + 2_000_000 + 200_000 + 500_000 + 800_000
	for _, model := range []string{"claude-opus-5-5", "claude-opus-5-5[1m]"} {
		usage := ClaudeUsageUsage(opusTurn(model, ""), new(0), "s", nil)
		if usage == nil || *usage.CostNanos != expected {
			t.Fatalf("%s usage = %+v", model, usage)
		}
		if *usage.Provenance != core.CostProvenanceEstimated || usage.Coverage != core.UsageCoverageComplete {
			t.Errorf("%s provenance/coverage = %v %v", model, *usage.Provenance, usage.Coverage)
		}
		if usage.Rate == nil || len(usage.Rate.Models) != 1 || usage.Rate.Models[0] != model || usage.Rate.Revision != RateTableClaude().Checked || usage.Rate.Source != RateTableClaude().Source {
			t.Errorf("%s rate = %+v", model, usage.Rate)
		}
	}
	for _, model := range []string{"claude-opus-5-5-high", "claude-opus-5-5-fast"} {
		usage := ClaudeUsageUsage(opusTurn(model, ""), new(0), "s", nil)
		if usage == nil || usage.CostNanos != nil || usage.Provenance != nil || usage.Coverage != core.UsageCoverageUntracked {
			t.Fatalf("%s = %+v", model, usage)
		}
		checkInt(t, "input", usage.Input, 1000)
		checkInt(t, "output", usage.Output, 100)
		checkInt(t, "cacheRead", usage.CacheRead, 1000)
		checkInt(t, "cacheWrite", usage.CacheWrite, 200)
	}
	reported := ClaudeUsageUsage(opusTurn("claude-opus-5-5", `,"costUSD":1.25`), new(0), "s", nil)
	if reported == nil || *reported.CostNanos != 1_250_000_000 || *reported.Provenance != core.CostProvenanceReported {
		t.Errorf("reported = %+v", reported)
	}
	zero := ClaudeUsageUsage(opusTurn("claude-opus-5-5[1m]", `,"costUSD":0`), new(0), "s", nil)
	if zero == nil || *zero.CostNanos != 0 || *zero.Provenance != core.CostProvenanceReported || zero.Coverage != core.UsageCoverageComplete {
		t.Errorf("zero = %+v", zero)
	}
}

func TestClaudeGlobalPriceAppliesOnlyWhenNoRegionWasAskedFor(t *testing.T) {
	t.Parallel()
	turn := func(geo string) string {
		return `{"type":"assistant","uuid":"u","sessionId":"s","message":{"id":"m","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":1000,"output_tokens":100,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":100},"output_tokens_details":{"thinking_tokens":45},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","inference_geo":"` + geo + `","speed":"standard"}}}`
	}
	const expected = 4_000_000 + 2_000_000 + 200_000 + 500_000 + 800_000
	for _, geo := range []string{"not_available", "global"} {
		usage := ClaudeUsageUsage(turn(geo), new(0), "s", nil)
		if usage == nil || *usage.CostNanos != expected || *usage.Provenance != core.CostProvenanceEstimated || usage.Coverage != core.UsageCoverageComplete {
			t.Fatalf("%s usage = %+v", geo, usage)
		}
	}
	// A US-only request costs more than the bundled list price, so it stays unpriced.
	if us := ClaudeUsageUsage(turn("us"), new(0), "s", nil); us == nil || us.CostNanos != nil || us.Coverage != core.UsageCoverageUntracked {
		t.Errorf("us = %+v", us)
	}
}

func TestCodexUsesCumulativeDeltasAndNotTheRepeatedReport(t *testing.T) {
	t.Parallel()
	log := fixtureText(t, "codex-rollout.jsonl")
	whole := CodexUsageOf(log, &core.TokenTotals{}, nil)
	if whole == nil {
		t.Fatal("no usage")
	}
	if *whole.CostNanos != 2_625_000 {
		t.Errorf("cost = %d", *whole.CostNanos)
	}
	checkInt(t, "total", whole.TotalTokens, 1590)
	checkInt(t, "input", whole.Input, 700)
	checkInt(t, "cacheRead", whole.CacheRead, 800)
	checkInt(t, "output", whole.Output, 90)
	checkInt(t, "reasoning", whole.Reasoning, 30)
	if whole.CacheWrite != nil {
		t.Errorf("cacheWrite = %d, want nil", *whole.CacheWrite)
	}
	if whole.Provenance == nil || *whole.Provenance != core.CostProvenanceEstimated {
		t.Errorf("provenance = %v", whole.Provenance)
	}
	resumed := CodexUsageOf(log, &core.TokenTotals{Input: 1000, CacheRead: 400, Output: 50, Reasoning: 20, Total: 1050}, nil)
	if resumed == nil || *resumed.CostNanos != 805_000 {
		t.Fatalf("resumed = %+v", resumed)
	}
	checkInt(t, "resumed total", resumed.TotalTokens, 540)
	if CodexUsageOf(log, nil, nil) != nil {
		t.Errorf("usage without a baseline")
	}
	reset := strings.ReplaceAll(log, `"total_tokens":1590`, `"total_tokens":10`)
	reset = strings.ReplaceAll(reset, `"input_tokens":1500`, `"input_tokens":1`)
	kept := CodexUsageOf(reset, &core.TokenTotals{}, nil)
	if kept == nil || *kept.CostNanos != 1_820_000 {
		t.Fatalf("kept = %+v", kept)
	}
	checkInt(t, "kept total", kept.TotalTokens, 1050)
	checkInt(t, "kept input", kept.Input, 600)
	checkInt(t, "kept cacheRead", kept.CacheRead, 400)
	checkInt(t, "kept output", kept.Output, 50)
	checkInt(t, "kept reasoning", kept.Reasoning, 20)
	if kept.Coverage != core.UsageCoveragePartial || *kept.Provenance != core.CostProvenanceEstimated {
		t.Errorf("kept coverage/provenance = %v %v", kept.Coverage, kept.Provenance)
	}
}

func TestCodexResumesFromZeroOnlyBeforeAnyTokenCount(t *testing.T) {
	t.Parallel()
	meta := `{"type":"session_meta","payload":{"id":"r","cwd":"/work","model":"gpt-5.3-codex"}}`
	if got := CodexBaseline(meta); got == nil || *got != core.TokenTotalsZero {
		t.Fatalf("baseline of a fresh rollout = %+v", got)
	}
	complete := meta + "\n" + `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":15}}}}`
	if got := CodexBaseline(complete); got == nil || got.Total != 15 {
		t.Fatalf("baseline of a counted rollout = %+v", got)
	}
	// A rollout that never reports reasoning has spent tokens, so it is no zero baseline.
	noReasoning := meta + "\n" + `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":5,"total_tokens":15}}}}`
	if CodexBaseline(noReasoning) != nil || CodexUsageOf(noReasoning, CodexBaseline(noReasoning), nil) != nil {
		t.Fatalf("baseline or usage without reasoning")
	}
	broken := meta + "\n" + `{"type":"event_msg","payload":{"type":"token_count","info":{}}}`
	if CodexBaseline(broken) != nil {
		t.Fatalf("baseline of a broken event")
	}
}

func TestCodexKeepsEachCounterWhenOneIsMissing(t *testing.T) {
	t.Parallel()
	partial := CodexUsageOf(fixtureText(t, "codex-partial.jsonl"), &core.TokenTotals{}, nil)
	if partial == nil {
		t.Fatal("no usage")
	}
	// Event missing reasoning: uncached 60 at $1.75, cache 40 at $0.175, output 10 at $14.
	if *partial.CostNanos != 252_000 {
		t.Errorf("cost = %d", *partial.CostNanos)
	}
	if *partial.Provenance != core.CostProvenanceEstimated || partial.Coverage != core.UsageCoveragePartial {
		t.Errorf("provenance/coverage = %v %v", *partial.Provenance, partial.Coverage)
	}
	checkInt(t, "input", partial.Input, 60)
	checkInt(t, "cacheRead", partial.CacheRead, 40)
	checkInt(t, "output", partial.Output, 10)
	checkInt(t, "reasoning", partial.Reasoning, 4)
	checkInt(t, "total", partial.TotalTokens, 110)
	if partial.CacheWrite != nil {
		t.Errorf("cacheWrite = %d, want nil", *partial.CacheWrite)
	}
	missingInput := `{"type":"turn_context","payload":{"model":"gpt-5.3-codex"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"cached_input_tokens":40,"output_tokens":10,"reasoning_output_tokens":4,"total_tokens":50}}}}`
	tokensOnly := CodexUsageOf(missingInput, &core.TokenTotals{}, nil)
	if tokensOnly == nil || tokensOnly.Input != nil {
		t.Fatalf("tokensOnly = %+v", tokensOnly)
	}
	checkInt(t, "cacheRead", tokensOnly.CacheRead, 40)
	checkInt(t, "output", tokensOnly.Output, 10)
	checkInt(t, "reasoning", tokensOnly.Reasoning, 4)
	checkInt(t, "total", tokensOnly.TotalTokens, 50)
	if tokensOnly.CostNanos != nil || tokensOnly.Provenance != nil || tokensOnly.Coverage != core.UsageCoveragePartial {
		t.Errorf("tokensOnly = %+v", tokensOnly)
	}
	filled := fixtureText(t, "codex-partial.jsonl") + "\n" + `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":160,"cached_input_tokens":40,"output_tokens":10,"reasoning_output_tokens":4,"total_tokens":170}}}}`
	later := CodexUsageOf(filled, &core.TokenTotals{}, nil)
	if later == nil {
		t.Fatal("no later usage")
	}
	checkInt(t, "later input", later.Input, 120)
	checkInt(t, "later cacheRead", later.CacheRead, 40)
	checkInt(t, "later output", later.Output, 10)
	checkInt(t, "later reasoning", later.Reasoning, 4)
	checkInt(t, "later total", later.TotalTokens, 170)
	if later.CostNanos == nil || *later.CostNanos != 357_000 || later.Coverage != core.UsageCoveragePartial {
		t.Errorf("later cost/coverage = %v %v", later.CostNanos, later.Coverage)
	}
}

func TestCodexPricesAModelChangeAndLeavesAnUnknownModelUntracked(t *testing.T) {
	t.Parallel()
	rates := NewRateTable(RateTableArgs{Provider: "test", Source: "https://example.test/codex", Checked: "codex-test", Models: map[string]ModelRate{
		"cheap": {Standard: Prices{Input: rat(t, "1"), Output: rat(t, "2"), CacheRead: rat(t, "1")}},
		"dear":  {Standard: Prices{Input: rat(t, "4"), Output: rat(t, "8"), CacheRead: rat(t, "1")}},
	}})
	count := func(model string, total int) string {
		n := strconv.Itoa(total)
		return `{"type":"turn_context","payload":{"model":"` + model + `"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":` + n + `,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0,"total_tokens":` + n + `}}}}`
	}
	usage := CodexUsageOf(count("cheap", 10)+"\n"+count("dear", 12), &core.TokenTotals{}, &rates)
	// 10 * $1 + 2 * $4, per million, * 1000 nanos.
	if usage == nil || *usage.CostNanos != 10_000+8_000 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Rate == nil || len(usage.Rate.Models) != 2 || usage.Rate.Models[0] != "cheap" || usage.Rate.Models[1] != "dear" || usage.Coverage != core.UsageCoverageComplete {
		t.Errorf("rate = %+v coverage = %v", usage.Rate, usage.Coverage)
	}
	unknown := CodexUsageOf(count("gpt-5.4", 10), &core.TokenTotals{}, &rates)
	if unknown == nil || unknown.CostNanos != nil {
		t.Fatalf("unknown = %+v", unknown)
	}
	checkInt(t, "unknown total", unknown.TotalTokens, 10)
	if unknown.Coverage != core.UsageCoverageUntracked {
		t.Errorf("coverage = %v", unknown.Coverage)
	}
}

func TestCursorResultHasNoUsage(t *testing.T) {
	t.Parallel()
	if (CursorAdapter{}).Usage(t.TempDir(), "chat", "name", 0, 0) != nil {
		t.Fatal("adapter reports usage")
	}
}

func TestSpendMarksPartialEstimatesAndIgnoresOpenAndNonAgentWork(t *testing.T) {
	t.Parallel()
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no America/New_York zone: %v", err)
	}
	spring := time.Date(2026, 3, 8, 3, 30, 0, 0, newYork)
	run := usageRun(t, spring)
	reported := usageOf(340_000_000)
	estimated := core.NewUsage(core.UsageArgs{CostNanos: new(120_000_000), Provenance: new(core.CostProvenanceEstimated), Coverage: new(core.UsageCoverageComplete)})
	// attemptAt is an attempt of stage that ended at the given time, or is still open when at is nil.
	attemptAt := func(stage string, ended *time.Time, usage *core.Usage) core.Attempt {
		if ended == nil {
			attempt := core.NewAttempt(core.AttemptArgs{StageID: stage, Number: 1, StartedAt: jsonx.Date{Time: spring}, Folder: stage})
			if stage != "approve" {
				attempt.Session = new("bl-" + stage)
			}
			attempt.Usage = usage
			return attempt
		}
		session := new("bl-" + stage)
		if stage == "approve" {
			session = nil
		}
		return attemptEnded(stage, *ended, session, usage)
	}
	at := spring
	run.Attempts = []core.Attempt{
		attemptAt("build", &at, &reported),
		attemptAt("spec", &at, &estimated),
		attemptAt("verify", &at, nil),
		attemptAt("approve", &at, nil),
		attemptAt("triage", nil, nil),
	}
	spend := core.CostSpendOf(run)
	if line := core.CostRunLine(spend); line == nil || *line != "$0.46 · partial · estimated · ended sessions" {
		t.Fatalf("run line = %v", line)
	}
	if got := core.CostAmount(core.CostToday([]core.Run{run}, newYork, spring)); got != "$0.46" {
		t.Fatalf("today = %q", got)
	}
	previousDay := time.Date(2026, 3, 7, 23, 30, 0, 0, newYork)
	endedPrevious := jsonx.Date{Time: previousDay}
	run.Attempts[0].EndedAt = &endedPrevious
	if today := core.CostToday([]core.Run{run}, newYork, spring); today.Nanos == nil || *today.Nanos != 120_000_000 {
		t.Fatalf("today after the previous day = %+v", today)
	}
	run.Attempts = []core.Attempt{attemptAt("build", &at, nil), attemptAt("spec", &at, nil)}
	if got := core.CostAmount(core.CostSpendOf(run)); got != "$0.00" {
		t.Fatalf("unpriced amount = %q", got)
	}
	if line := core.CostRunLine(core.CostSpendOf(run)); line != nil && strings.Contains(*line, "$0.00") {
		t.Fatalf("unpriced run line = %q", *line)
	}
	run.Attempts = []core.Attempt{attemptAt("approve", &at, nil)}
	if line := core.CostRunLine(core.CostSpendOf(run)); line != nil {
		t.Fatalf("approval-only run line = %q", *line)
	}
	if got := core.CostAmount(core.CostToday([]core.Run{run}, newYork, spring)); got != "$0.00" {
		t.Fatalf("approval-only today = %q", got)
	}
}

func TestLabelsCoverReportedEstimatedPartialAndUnknown(t *testing.T) {
	t.Parallel()
	now := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)
	ended := func(usage *core.Usage) core.Attempt {
		return attemptEnded("build", now, new("bl"), usage)
	}
	reported := usageOf(340_000_000)
	zero := usageOf(0)
	estimated := core.NewUsage(core.UsageArgs{CostNanos: new(120_000_000), Input: new(4), TotalTokens: new(4),
		Provenance: new(core.CostProvenanceEstimated), Coverage: new(core.UsageCoverageComplete),
		Rate: &core.RateProvenance{Revision: "r", Source: "s", Models: []string{"m"}}})
	partial := core.NewUsage(core.UsageArgs{CostNanos: new(120_000_000), TotalTokens: new(4),
		Provenance: new(core.CostProvenanceEstimated), Coverage: new(core.UsageCoveragePartial), Rate: estimated.Rate})
	tokensOnly := core.NewUsage(core.UsageArgs{Input: new(9), TotalTokens: new(9)})
	cases := []struct {
		name, want string
		usage      *core.Usage
	}{
		{"reported", "1m 0s · $0.34", &reported},
		{"zero", "1m 0s · $0.00", &zero},
		{"estimated", "1m 0s · $0.12 · estimated", &estimated},
	}
	for _, c := range cases {
		if got := core.CostSession(ended(c.usage), true, now); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
	if got := core.CostAttemptLabel(&partial); got != "$0.12 · partial · estimated" {
		t.Errorf("partial = %q", got)
	}
	if got := core.CostAttemptLabel(&tokensOnly); got != "cost not tracked" {
		t.Errorf("tokens only = %q", got)
	}
	if got := core.CostAttemptLabel(nil); got != "cost not tracked" {
		t.Errorf("nil label = %q", got)
	}
	if got := core.CostTokenDetail(tokensOnly); got == nil || *got != "9 in · 9 total" {
		t.Errorf("tokens detail = %v", got)
	}
	if got := core.CostTokenDetail(estimated); got == nil || *got != "4 in · 4 total" {
		t.Errorf("token detail = %v", got)
	}
	if got := core.CostTokenDetail(usageOf(1)); got != nil {
		t.Errorf("cost-only detail = %q", *got)
	}
	if !core.CostExplainsEstimate(&estimated) || !core.CostExplainsEstimate(&partial) {
		t.Errorf("estimate not explained")
	}
	if core.CostEstimateExplanation != "API-equivalent usage cost from the bundled rate table; not subscription charges or invoices." {
		t.Errorf("explanation = %q", core.CostEstimateExplanation)
	}
	open := ended(nil)
	open.EndedAt = nil
	if got := core.CostSession(open, true, now); got != "live" {
		t.Errorf("open = %q", got)
	}
	gate := ended(nil)
	gate.Session = nil
	if got := core.CostSession(gate, false, now); got != "1m 0s" {
		t.Errorf("gate = %q", got)
	}
}
