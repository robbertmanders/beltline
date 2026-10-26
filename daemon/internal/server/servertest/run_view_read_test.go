// The run view cases: the shared projection for each layout fixture.
package servertest

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// runViewPath is the run page's path for id, with the id escaped as a path segment.
func runViewPath(id string) string { return "/api/v1/views/run/" + EncodeSegment(id) }

// decodeRunView decodes a run view reply's body.
func decodeRunView(t *testing.T, reply Reply) api.RunViewResponse {
	t.Helper()
	var response api.RunViewResponse
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("run view: %v", err)
	}
	return response
}

// notFound is the 404 every missing run view answers with.
var notFound = Expect{Status: http.StatusNotFound, ContentType: "application/json"}

// TestRunViewUsesSharedProjectionForLayoutFixtures ports
// RunReadTests.runViewUsesSharedProjectionForWidthsAndLayoutFixtures.
func TestRunViewUsesSharedProjectionForLayoutFixtures(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	run := f.Coordinator.ReadRun(f.ActionRunID)
	if run == nil {
		t.Fatal("the action run is gone")
	}
	reply := f.Do(Request{Method: "GET", Path: runViewPath(run.ID), Token: ContractToken})
	if reply.Status != http.StatusOK {
		t.Fatalf("status %d", reply.Status)
	}
	snapshot := f.Coordinator.ReadRunViewSnapshot(run.ID, nil)
	if snapshot == nil {
		t.Fatal("no snapshot")
	}
	if !bytes.Equal(reply.Body, mustMarshal(t, api.NewRunViewResponse(*snapshot))) {
		t.Fatal("the response is not the shared projection")
	}
	response := decodeRunView(t, reply)
	if len(response.Boxes.Map.Nodes) != len(run.Stages)+1 {
		t.Fatal("the boxes do not hold the trigger and every stage")
	}
	if !reflect.DeepEqual(response.Actions, f.Coordinator.RunActions(*run)) {
		t.Fatal("actions differ from the coordinator's")
	}

	type layoutCase struct {
		name      string
		predicate func(core.Run) bool
	}
	cases := []layoutCase{
		{"straight", func(r core.Run) bool { return len(r.Layout().Forks) == 0 }},
		{"forked", func(r core.Run) bool { return len(r.Layout().Forks) > 0 && !r.Layout().Nested() }},
		{"nested", func(r core.Run) bool { return r.Layout().Nested() }},
		{"held", func(r core.Run) bool { return r.State == core.RunStateHeld }},
		{"parked", func(r core.Run) bool { return r.State == core.RunStateParked }},
	}
	runs := f.Coordinator.ReadRuns()
	for _, layout := range cases {
		var example *core.Run
		for i := range runs {
			if layout.predicate(runs[i]) {
				example = &runs[i]
				break
			}
		}
		if example == nil {
			t.Fatalf("fixture needs a %s run", layout.name)
		}
		reply := f.Do(Request{Method: "GET", Path: runViewPath(example.ID), Token: ContractToken})
		if reply.Status != http.StatusOK {
			t.Fatalf("%s: status %d", layout.name, reply.Status)
		}
		snapshot := f.Coordinator.ReadRunViewSnapshot(example.ID, nil)
		if snapshot == nil {
			t.Fatalf("%s: no snapshot", layout.name)
		}
		if !bytes.Equal(reply.Body, mustMarshal(t, api.NewRunViewResponse(*snapshot))) {
			t.Fatalf("%s: the response is not the shared projection", layout.name)
		}
		CheckGolden(t, "run view "+layout.name, "views-run-"+layout.name+".json", reply, f.Root)
	}

	f.Coordinator.AppendRunForContract(SplitRun(ContractNow))
	splitReply := f.Do(Request{Method: "GET", Path: runViewPath("split/view"), Token: ContractToken})
	if splitReply.Status != http.StatusOK {
		t.Fatalf("split: status %d", splitReply.Status)
	}
	split := decodeRunView(t, splitReply)
	if !reflect.DeepEqual(split.Boxes.TakenRoutes, []string{"scope:simple", "scope:thorough"}) {
		t.Fatalf("split: taken routes %v", split.Boxes.TakenRoutes)
	}
	if node := split.Boxes.Node("simple"); node == nil || node.State != core.RunBoxesStateRunning {
		t.Fatal("split: the simple stage is not running")
	}
	if node := split.Boxes.Node("thorough"); node == nil || node.State != core.RunBoxesStateTodo {
		t.Fatal("split: the thorough stage is not todo")
	}
	CheckGolden(t, "split run view", "views-run-split.json", splitReply, f.Root)

	missing := f.Do(Request{Method: "GET", Path: runViewPath("no-such-run"), Token: ContractToken})
	if missing.Status != http.StatusNotFound {
		t.Fatalf("missing: status %d", missing.Status)
	}
	CheckGoldenAs(t, notFound, "missing run view", "error-not-found.json", missing, f.Root)

	if err := store.NewStorage(store.Paths{Root: f.Root}).RecordErased("erased-read-run", 0); err != nil {
		t.Fatalf("record erased: %v", err)
	}
	erased := f.Do(Request{Method: "GET", Path: runViewPath("erased-read-run"), Token: ContractToken})
	if erased.Status != http.StatusNotFound {
		t.Fatalf("erased: status %d", erased.Status)
	}
	CheckGoldenAs(t, notFound, "erased run view", "error-not-found.json", erased, f.Root)
}
