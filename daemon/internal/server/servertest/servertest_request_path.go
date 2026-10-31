// The concrete request path of each contract case, with its path segments percent-encoded.

package servertest

import (
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// RequestPath is the path the contract tests request for endpoint on f.
func RequestPath(f *Fixture, endpoint routes.Endpoint) string {
	firstBelt := func() string {
		if belts := f.Coordinator.Belts(); len(belts) > 0 {
			return belts[0].Belt.ID
		}
		return "issues"
	}
	path := endpoint.Path
	switch endpoint.Case {
	case "auth":
		return "/auth"
	case "placeholders":
		return "/api/v1/placeholders?belt=" + EncodeSegment(firstBelt())
	case "viewsInsightsEvolution":
		return "/api/v1/views/insights/evolution?belt=" + EncodeSegment(firstBelt())
	case "beltArchive", "beltUnarchive", "beltPause", "beltAfk", "beltShadowSave", "beltShadowAccept", "beltShadowDiscard":
		return strings.ReplaceAll(path, ":id", EncodeSegment(firstBelt()))
	case "agentArchive", "agentUnarchive":
		return strings.ReplaceAll(path, ":id", EncodeSegment("triage"))
	case "runDecide", "runResume", "runAnswer", "runAbort", "runDrop", "runPark", "runUnpark", "runPause",
		"runResumeRun", "runRetryContinuation", "runStartNow", "runContinueSession", "runAdjustStage", "runAdjustSwitch",
		"runAfk", "runErase", "runOverruleAfk":
		return strings.ReplaceAll(path, ":id", EncodeSegment(f.Coordinator.Runs()[0].ID))
	case "run":
		runs := f.Coordinator.Runs()
		run := runs[0]
		for _, candidate := range runs {
			if len(candidate.Attempts) > 0 {
				run = candidate
				break
			}
		}
		return "/api/v1/runs/" + EncodeSegment(run.ID)
	case "runAttemptOutput", "runAttemptArtifacts":
		path = strings.ReplaceAll(path, ":id", EncodeSegment(f.ActionRunID))
		return strings.ReplaceAll(path, ":attempt", EncodeSegment(f.ActionAttemptID))
	case "viewsRun":
		return "/api/v1/views/run/" + EncodeSegment(f.ActionRunID)
	case "viewsRunSessions":
		return "/api/v1/views/run/" + EncodeSegment(f.ActionRunID) + "/sessions"
	case "belt":
		return "/api/v1/belts/" + EncodeSegment(f.Coordinator.Belts()[0].Belt.ID)
	case "agent":
		return "/api/v1/agents/" + EncodeSegment("triage")
	case "models", "modelRefresh", "modelAllow":
		return strings.ReplaceAll(path, ":runner", "pi")
	case "libraryItem":
		return libraryItemPath
	case "libraryBelt":
		return libraryBeltPath
	case "libraryAgent":
		return libraryAgentPath
	}
	return path
}

// EncodeSegment percent-encodes everything but RFC 3986's unreserved characters.
// addingPercentEncoding with that set (UTF-8 bytes, upper-case hex).
func EncodeSegment(value string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
