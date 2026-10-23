package coordinator

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// ModelActionDecision says whether one model command may run, and the daemon's
// reason when it may not.
type ModelActionDecision struct {
	Available     bool    `json:"available"`
	RefusalReason *string `json:"refusalReason"`
}

// ModelCandidateActionDecision is one candidate's allow and remove availability.
type ModelCandidateActionDecision struct {
	ID            string  `json:"id"`
	Allowed       bool    `json:"allowed"`
	Available     bool    `json:"available"`
	RefusalReason *string `json:"refusalReason"`
}

// ModelRunnerActions are the daemon-derived decisions for a runner's Allowed models card.
type ModelRunnerActions struct {
	Refresh    ModelActionDecision            `json:"refresh"`
	Candidates []ModelCandidateActionDecision `json:"candidates"`
}

// ModelRunnerResource is a runner's model resource: its catalog state, the
// currently admitted choices and the saved permissions that are no longer in the catalog.
type ModelRunnerResource struct {
	Runner     core.Backend               `json:"runner"`
	Executable string                     `json:"executable"`
	Catalog    *core.ModelCatalogSnapshot `json:"catalog"`
	Usable     bool                       `json:"usable"`
	Loading    bool                       `json:"loading"`
	Error      *string                    `json:"error"`
	// AllowedModels are the saved permissions for the runner.
	AllowedModels []string           `json:"allowedModels"`
	Choices       []core.ModelChoice `json:"choices"`
	// UnavailableModels are the saved permissions no longer in the catalog.
	UnavailableModels []string `json:"unavailableModels"`
	// Thinking are the thinking levels this runner accepts, labelled by the daemon.
	Thinking []core.ThinkingChoice `json:"thinking"`
	// ThinkingNote says why Thinking is off for a runner that has no thinking setting.
	ThinkingNote *string `json:"thinkingNote"`
	// Listing is every catalog model matching the search, with its label and whether it is allowed.
	Listing []ModelListing     `json:"listing"`
	Actions ModelRunnerActions `json:"actions"`
}

// ModelListing is one catalog model as the Allowed models list shows it.
type ModelListing struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Allowed bool   `json:"allowed"`
}

// ReadModelRunner reads a runner's models. It never writes settings. With no usable
// catalog it starts a non-forced discovery and answers at once, with loading set.
// query filters the listing with the matcher the old card used.
// Lock: exported entry point; takes c.mu. Spawns: RequestDiscovery.
func (c *Coordinator) ReadModelRunner(runner core.Backend, query string) ModelRunnerResource {
	c.Lock()
	defer c.EndTurn()
	return c.readModelRunnerNow(runner, query)
}

// readModelRunnerNow is ReadModelRunner with c.mu held.
// Lock: runs with c.mu held. Spawns: RequestDiscovery.
func (c *Coordinator) readModelRunnerNow(runner core.Backend, query string) ModelRunnerResource {
	var snapshot *core.ModelCatalogSnapshot
	if stored, ok := c.settings.ModelCatalogs[string(runner)]; ok {
		snapshot = &stored
	}
	executable := runner.Executable(c.env)
	usable := snapshot != nil && snapshot.IsUsable(runner, executable)
	if !usable {
		c.modelCatalogs.RequestDiscovery(runner, c)
	}
	allowed := sortedNames(c.settings.AllowedModels[string(runner)])
	listing := []ModelListing{}
	if usable {
		for _, model := range snapshot.Models {
			if model.Matches(query) {
				listing = append(listing, ModelListing{
					ID: model.ID, Label: model.DisplayLabel(), Allowed: slices.Contains(allowed, model.ID),
				})
			}
		}
	}
	allowance := core.ModelAllowance{Identifiers: map[core.Backend]map[string]struct{}{runner: modelIdentifierSet(allowed)}}
	unavailable := allowance.UnavailableIdentifiers(runner, snapshot, executable)
	var candidateIDs []string
	if snapshot != nil {
		for _, model := range snapshot.Models {
			candidateIDs = append(candidateIDs, model.ID)
		}
	}
	candidateIDs = append(candidateIDs, unavailable...)
	actions := ModelRunnerActions{Refresh: ModelActionDecision{Available: true}, Candidates: []ModelCandidateActionDecision{}}
	for _, id := range modelUniqueSorted(candidateIDs) {
		if slices.Contains(allowed, id) {
			actions.Candidates = append(actions.Candidates, ModelCandidateActionDecision{ID: id, Allowed: true, Available: true})
			continue
		}
		inCatalog := usable && catalogListsModel(*snapshot, id)
		decision := ModelCandidateActionDecision{ID: id, Allowed: false, Available: inCatalog}
		if !inCatalog {
			reason := fmt.Sprintf("%s is unavailable in catalog for %s. Refresh its catalog in Settings → Allowed models.", id, runner.Label())
			decision.RefusalReason = &reason
		}
		actions.Candidates = append(actions.Candidates, decision)
	}
	thinking := []core.ThinkingChoice{}
	for _, level := range core.StageAdjustSupportedEffort(runner) {
		thinking = append(thinking, core.ThinkingChoice{ID: level, Label: level.Label()})
	}
	choices := allowance.Choices(runner, snapshot, executable, "")
	sort.SliceStable(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	var errText *string
	if text, ok := c.modelCatalogs.errors[runner]; ok {
		errText = &text
	}
	_, loading := c.modelCatalogs.loading[runner]
	return ModelRunnerResource{
		Runner: runner, Executable: executable, Catalog: snapshot, Usable: usable,
		Loading: loading, Error: errText, AllowedModels: allowed, Choices: choices,
		UnavailableModels: unavailable, Thinking: thinking, ThinkingNote: runner.ThinkingNote(),
		Listing: listing, Actions: actions,
	}
}

// modelIdentifierSet is the set of a list of identifiers. Lock: none (pure).
func modelIdentifierSet(identifiers []string) map[string]struct{} {
	out := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		out[identifier] = struct{}{}
	}
	return out
}

// modelUniqueSorted is the distinct values of list, sorted. Lock: none (pure).
func modelUniqueSorted(list []string) []string {
	return slices.Sorted(maps.Keys(modelIdentifierSet(list)))
}

// RefreshModelCatalog forces a catalog refresh and returns the completed resource.
// A discovery failure stays in the resource's Error; the last catalog is kept.
// Lock: exported entry point; takes c.mu. Suspends: modelCatalogs.Refresh.
func (c *Coordinator) RefreshModelCatalog(ctx context.Context, runner core.Backend) ModelRunnerResource {
	c.Lock()
	defer c.EndTurn()
	c.modelCatalogs.Refresh(ctx, runner, c, true)
	return c.readModelRunnerNow(runner, "")
}

// SetModelAllowedAndRead returns the runner's resource after allowing or removing a model.
// Lock: exported entry point; takes c.mu. Writes: publishes settings.
func (c *Coordinator) SetModelAllowedAndRead(identifier string, runner core.Backend, allowed bool) (ModelRunnerResource, error) {
	c.Lock()
	defer c.EndTurn()
	if err := c.setModelAllowedNow(identifier, runner, allowed); err != nil {
		return ModelRunnerResource{}, err
	}
	return c.readModelRunnerNow(runner, ""), nil
}
