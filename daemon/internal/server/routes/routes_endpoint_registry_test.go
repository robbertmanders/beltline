// Inventory checks of the endpoint registry: it lists every endpoint of the contract.

package routes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// endpointCount is the number of endpoints the table names: the registry's cases less its
// factory and marketplace endpoints, which library repositories replace with ten of their own.
const endpointCount = 111

// TestEndpointRegistryCount checks the registry lists every endpoint of the contract.
func TestEndpointRegistryCount(t *testing.T) {
	t.Parallel()
	if got := len(Endpoints); got != endpointCount {
		t.Fatalf("registry has %d endpoints, want %d", got, endpointCount)
	}
}

// TestEndpointGoldensExist checks every golden name the registry lists is a file in testdata/golden.
func TestEndpointGoldensExist(t *testing.T) {
	t.Parallel()
	dir := testkit.Testdata(t, "golden")
	for _, endpoint := range Endpoints {
		if _, err := os.Stat(filepath.Join(dir, endpoint.GoldenName)); err != nil {
			t.Errorf("endpoint %s: golden %s: %v", endpoint.Case, endpoint.GoldenName, err)
		}
	}
}

// TestEndpointCasesAreUnique checks no two endpoints share a case name, so contract coverage can key on it.
func TestEndpointCasesAreUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, endpoint := range Endpoints {
		if seen[endpoint.Case] {
			t.Errorf("duplicate endpoint case %s", endpoint.Case)
		}
		seen[endpoint.Case] = true
	}
}

// TestAreaRegistrationsAreUniqueAndOrdered checks the table RegisterAll walks: each area once, in
// the order of AllEndpointAreas, so no area can be registered twice.
func TestAreaRegistrationsAreUniqueAndOrdered(t *testing.T) {
	t.Parallel()
	position := map[EndpointArea]int{}
	for i, area := range AllEndpointAreas {
		position[area] = i
	}
	last := -1
	seen := map[EndpointArea]bool{}
	for _, reg := range areaRegistrations {
		if seen[reg.area] {
			t.Errorf("area %s is in the table twice", reg.area)
		}
		seen[reg.area] = true
		index, ok := position[reg.area]
		if !ok {
			t.Errorf("area %s is not an EndpointArea", reg.area)
			continue
		}
		if index <= last {
			t.Errorf("area %s is out of order", reg.area)
		}
		last = index
	}
}

// TestEndpointsInListsEachAreasRegistryEntries checks EndpointsIn against the table: every entry belongs
// to its area, and the areas together list every entry once.
func TestEndpointsInListsEachAreasRegistryEntries(t *testing.T) {
	t.Parallel()
	total := 0
	for _, area := range AllEndpointAreas {
		for _, endpoint := range EndpointsIn(area) {
			if endpoint.Area != area {
				t.Errorf("%s is listed under %s", endpoint.Case, area)
			}
			total++
		}
	}
	if total != len(Endpoints) {
		t.Errorf("areas list %d entries, the registry has %d", total, len(Endpoints))
	}
}
