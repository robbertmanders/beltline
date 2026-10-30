// The endpoint registry: its types, the area registration, and the registration of every area on one
// mux. The table itself is in routes_endpoint_registry_table.go.

package routes

import (
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/kit/launch"
)

// EndpointMethod is an HTTP method available to production routes.
type EndpointMethod string

// The EndpointMethod values.
const (
	EndpointMethodGet  EndpointMethod = "GET"
	EndpointMethodPost EndpointMethod = "POST"
)

// EndpointArea is the API surface an endpoint belongs to; each area registers
// its own routes.
type EndpointArea string

// The EndpointArea values, in order.
const (
	AreaAuthVersion   EndpointArea = "authVersion"
	AreaStateSettings EndpointArea = "stateSettings"
	AreaRuns          EndpointArea = "runs"
	AreaViews         EndpointArea = "views"
	AreaTerminals     EndpointArea = "terminals"
	AreaBelts         EndpointArea = "belts"
	AreaAgents        EndpointArea = "agents"
	AreaTools         EndpointArea = "tools"
	AreaModels        EndpointArea = "models"
	AreaAssistant     EndpointArea = "assistant"
	AreaGlobal        EndpointArea = "global"
	AreaLibrary       EndpointArea = "library"
	AreaArtifacts     EndpointArea = "artifacts"
	AreaDesktop       EndpointArea = "desktop"
	AreaEvents        EndpointArea = "events"
)

// AllEndpointAreas is every EndpointArea value, in order.
var AllEndpointAreas = []EndpointArea{
	AreaAuthVersion, AreaStateSettings, AreaRuns, AreaViews, AreaTerminals, AreaBelts, AreaAgents,
	AreaTools, AreaModels, AreaAssistant, AreaGlobal, AreaLibrary, AreaArtifacts,
	AreaDesktop, AreaEvents,
}

// Endpoint is one entry of the registry: the descriptor the contract coverage
// reads. Case is the case name (its contract case); GoldenName is the file
// in daemon/testdata/golden that records the endpoint's response.
type Endpoint struct {
	Case       string
	Method     EndpointMethod
	Path       string
	Area       EndpointArea
	GoldenName string
}

// EndpointsIn is the registry entries of one area,
// in registry order.
func EndpointsIn(area EndpointArea) []Endpoint {
	var out []Endpoint
	for _, endpoint := range Endpoints {
		if endpoint.Area == area {
			out = append(out, endpoint)
		}
	}
	return out
}

// areaRegistration is one area's registration on the mux.
type areaRegistration struct {
	area     EndpointArea
	register func(mux *http.ServeMux, deps Dependencies)
}

// areaRegistrations is the table of the areas RegisterAll registers, in
// allCases order. An area appears once: RegisterAll panics on a repeat. A route
// unit adds its area here when its handlers land (in place, in order),
// and the contract coverage then treats its cases as served.
// The terminal and events areas are not here: the server adds their WebSocket upgrades.
var areaRegistrations = []areaRegistration{
	{AreaAuthVersion, registerAuthVersion},
	{AreaStateSettings, func(mux *http.ServeMux, deps Dependencies) {
		registerStateSettings(mux, deps.Coordinator)
	}},
	{AreaRuns, func(mux *http.ServeMux, deps Dependencies) {
		registerRuns(mux, deps.Coordinator, deps.ArtifactSigner)
	}},
	{AreaViews, func(mux *http.ServeMux, deps Dependencies) {
		registerViews(mux, deps.Coordinator)
	}},
	{AreaBelts, func(mux *http.ServeMux, deps Dependencies) {
		registerBelts(mux, deps.Coordinator)
	}},
	{AreaAgents, func(mux *http.ServeMux, deps Dependencies) {
		registerAgents(mux, deps.Coordinator)
	}},
	{AreaTools, func(mux *http.ServeMux, deps Dependencies) {
		registerTools(mux, deps.Coordinator)
	}},
	{AreaModels, func(mux *http.ServeMux, deps Dependencies) {
		registerModels(mux, deps.Coordinator)
	}},
	{AreaAssistant, func(mux *http.ServeMux, deps Dependencies) {
		registerAssistant(mux, deps.Coordinator)
	}},
	{AreaGlobal, func(mux *http.ServeMux, deps Dependencies) {
		registerGlobal(mux, deps.Coordinator)
	}},
	{AreaLibrary, func(mux *http.ServeMux, deps Dependencies) {
		registerLibrary(mux, deps.Coordinator)
	}},
	{AreaArtifacts, func(mux *http.ServeMux, deps Dependencies) {
		registerArtifacts(mux, deps.Coordinator, deps.ArtifactSigner)
	}},
	{AreaDesktop, registerDesktop},
}

// RegisterAll is the register all:
// every area registers its routes on the mux, in area order. The terminal and
// events areas register nothing here; the server adds their WebSocket upgrades.
// It panics when an area is registered twice (the table lists each area once).
func RegisterAll(mux *http.ServeMux, deps Dependencies) {
	seen := map[EndpointArea]bool{}
	for _, reg := range areaRegistrations {
		if seen[reg.area] {
			panic("routes: area " + string(reg.area) + " registered twice")
		}
		seen[reg.area] = true
		reg.register(mux, deps)
	}
}

// Dependencies is what the registration of every area needs, as the
// `add(to:)` parameters.
type Dependencies struct {
	Token   string
	Version string
	// Coordinator is nil when the server runs without one; the routes that need
	// it then answer with ServiceUnavailable (ReadRouteSupportUnavailable).
	Coordinator *coordinator.Coordinator
	// ArtifactSigner signs artifact links.
	ArtifactSigner ArtifactLinkSigner
	// DesktopCommandRunner runs the desktop commands (Finder, editors, open).
	DesktopCommandRunner shell.Runner
	// Platform says which system's desktop commands to run and report; the zero value is this one's.
	Platform launch.Platform
	// ApplicationLookup reports whether a desktop application is installed.
	ApplicationLookup DesktopApplicationLookup
	// SessionLookup reports whether a tmux session is available; nil when unset.
	SessionLookup DesktopSessionLookup
}

// registerAuthVersion is the register auth version.
// The auth route itself is registered by server (AddAuthRoute), which imports
// this package; only the version route is registered here.
func registerAuthVersion(mux *http.ServeMux, deps Dependencies) {
	AddVersion(mux, deps.Version, deps.Platform)
}

// registerStateSettings is the register state settings:
// the state read, the settings read and the settings patch.
func registerStateSettings(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddState(mux, c)
	AddSettingsRead(mux, c)
	AddSettingsPatch(mux, c)
}

// registerRuns is the register runs:
// the runs area's endpoints in registry order.
func registerRuns(mux *http.ServeMux, c *coordinator.Coordinator, signer ArtifactLinkSigner) {
	AddRunList(mux, c)
	AddRunDetail(mux, c)
	AddRunAttemptOutput(mux, c)
	AddRunAttemptArtifacts(mux, c, signer)
	AddRunDecide(mux, c)
	AddRunResume(mux, c)
	AddRunAnswer(mux, c)
	AddRunAbort(mux, c)
	AddRunDrop(mux, c)
	AddRunPark(mux, c)
	AddRunUnpark(mux, c)
	AddRunPause(mux, c)
	AddRunResumeRun(mux, c)
	AddRunRetryContinuation(mux, c)
	AddRunStartNow(mux, c)
	AddRunContinueSession(mux, c)
	AddRunAdjustStage(mux, c)
	AddRunAdjustSwitch(mux, c)
	AddRunAfk(mux, c)
	AddRunErase(mux, c)
	AddRunOverruleAfk(mux, c)
	AddTasks(mux, c)
}

// registerViews is the run page
// and its sessions, the Overview, Work, Insights, waiting, and the AFK decision log and its review.
func registerViews(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddViewOverview(mux, c)
	AddViewWork(mux, c)
	AddViewInsights(mux, c)
	AddViewInsightsEvolution(mux, c)
	AddViewRun(mux, c)
	AddViewRunSessions(mux, c)
	AddViewWaiting(mux, c)
	AddAfkDecisions(mux, c)
	AddAfkReview(mux, c)
}

// registerBelts is the register belts.
func registerBelts(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddBeltList(mux, c)
	AddBeltDetail(mux, c)
	AddBeltDraft(mux, c)
	AddBeltNew(mux, c)
	AddBeltCopy(mux, c)
	AddBeltUpdateStage(mux, c)
	AddBeltUpdateAllStages(mux, c)
	AddBeltSave(mux, c)
	AddBeltArchive(mux, c)
	AddBeltPause(mux, c)
	AddBeltAfk(mux, c)
	AddBeltShadow(mux, c)
	AddBeltStageTemplate(mux, c)
	AddBeltContext(mux, c)
	AddPlaceholders(mux, c)
}

// registerAgents is the register agents.
func registerAgents(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddAgentList(mux, c)
	AddAgentDetail(mux, c)
	AddAgentNew(mux, c)
	AddAgentSave(mux, c)
	AddAgentDraftPresentation(mux, c)
	AddAgentDraftSwitchRunner(mux, c)
	AddAgentArchive(mux, c)
}

// registerTools is the register tools.
func registerTools(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddTools(mux, c)
}

// registerModels is the register models.
func registerModels(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddModelsList(mux, c)
	AddModelsRefresh(mux, c)
	AddModelsAllow(mux, c)
}

// registerAssistant is the register assistant.
func registerAssistant(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddAssistantRead(mux, c)
	AddAssistantConfiguration(mux, c)
	AddAssistantStart(mux, c)
	AddAssistantStop(mux, c)
}

// registerGlobal is the register global.
func registerGlobal(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddGlobalPause(mux, c)
	AddGlobalAfk(mux, c)
	AddGlobalCheckNow(mux, c)
	AddGlobalQuit(mux, c)
}

// registerLibrary is the library repositories' routes.
func registerLibrary(mux *http.ServeMux, c *coordinator.Coordinator) {
	AddLibraryRepos(mux, c)
	AddLibraryItem(mux, c)
	AddLibraryRefresh(mux, c)
	AddLibraryBackup(mux, c)
	AddLibraryRestore(mux, c)
	AddLibraryInstall(mux, c)
	AddLibraryShare(mux, c)
}

// registerArtifacts registers the artifacts case of the endpoint areas:
// `ArtifactRoutes.add(to:coordinator:signer:)`.
func registerArtifacts(mux *http.ServeMux, c *coordinator.Coordinator, signer ArtifactLinkSigner) {
	AddArtifacts(mux, c, signer)
}

// registerDesktop registers the desktop case of the endpoint areas:
// `DesktopRoutes.add(to:coordinator:runner:applicationLookup:sessionLookup:)`.
func registerDesktop(mux *http.ServeMux, deps Dependencies) {
	AddDesktop(mux, deps.Coordinator, deps.DesktopCommandRunner, deps.Platform, deps.ApplicationLookup, deps.SessionLookup)
}
