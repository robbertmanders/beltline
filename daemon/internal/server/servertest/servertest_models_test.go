// The pi runner's
// resource (checkShape's .models arm) and a runner without a catalog, which is an explicit null
// (modelsWithoutACatalogEmitAnExplicitNull).
package servertest

import (
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

func TestModelsAnswerThePiRunnersResourceAsTheCoordinatorReadsIt(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	reply := f.Do(Request{Method: "GET", Path: "/api/v1/models/pi", Token: ContractToken})
	requireStatus(t, reply, 200)
	resource := jsonxModelRunner(t, reply)
	if resource.Executable != "pi" || !resource.Usable || resource.Catalog == nil {
		t.Fatalf("pi resource: executable %q usable %t catalog %v", resource.Executable, resource.Usable, resource.Catalog)
	}
	if resource.Loading || resource.Error != nil {
		t.Fatalf("pi resource is loading (%t) or failed (%v)", resource.Loading, resource.Error)
	}
	if !slices.IsSorted(resource.AllowedModels) {
		t.Fatalf("allowed models are not sorted: %v", resource.AllowedModels)
	}
	ids := make([]string, 0, len(resource.Choices))
	for _, choice := range resource.Choices {
		ids = append(ids, choice.ID)
	}
	if !slices.IsSorted(ids) {
		t.Fatalf("choices are not sorted by id: %v", ids)
	}
	if !slices.IsSorted(resource.UnavailableModels) {
		t.Fatalf("unavailable models are not sorted: %v", resource.UnavailableModels)
	}

	source, err := routes.JSONEncode(f.Coordinator.ReadModelRunner(core.BackendPi, ""))
	if err != nil {
		t.Fatalf("models: %v", err)
	}
	if string(reply.Body) != string(source) {
		t.Fatalf("pi resource differs from the coordinator's read:\n%s\n%s", reply.Body, source)
	}
}

func TestModelsWithoutACatalogEmitAnExplicitNull(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{Empty: true})
	reply := f.Do(Request{Method: "GET", Path: "/api/v1/models/copilot", Token: ContractToken})
	requireStatus(t, reply, 200)
	resource := jsonxModelRunner(t, reply)
	if resource.Catalog != nil || resource.Usable || len(resource.Choices) != 0 || len(resource.UnavailableModels) != 0 {
		t.Fatalf("a runner without a catalog answered %+v", resource)
	}
	if !strings.Contains(reply.Text(), `"catalog":null`) {
		t.Fatal("a missing catalog is not an explicit null")
	}
	CheckGolden(t, "models without a catalog", "models-empty.json", reply, f.Root)
}
