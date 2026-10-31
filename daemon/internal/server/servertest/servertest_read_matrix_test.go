// The read matrix: every read is authenticated, checks Host and Origin before lookup, and answers a
// missing resource the same way. It runs over the cases this harness serves.

package servertest

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// invalidToken is a bearer token that is not the contract token.
const invalidToken = "invalid-token"

// readMatrixSkipped are the GET endpoints the matrix leaves out: the auth redirect (no bearer),
// the artifact, the terminal and event upgrades, and the artifact and desktop areas (their own tests).
var readMatrixSkipped = map[string]bool{"auth": true, "artifact": true, "terminals": true, "events": true}

// servedGetEndpoints are the GET registry entries whose cases this harness serves, in registry order.
func servedGetEndpoints() []routes.Endpoint {
	var out []routes.Endpoint
	served := map[string]bool{}
	for _, c := range exercisedCases {
		served[c] = true
	}
	for _, endpoint := range routes.Endpoints {
		if endpoint.Method == routes.EndpointMethodGet && served[endpoint.Case] && !readMatrixSkipped[endpoint.Case] &&
			endpoint.Area != routes.AreaArtifacts && endpoint.Area != routes.AreaDesktop {
			out = append(out, endpoint)
		}
	}
	return out
}

// goldenBytes is the exact content of a golden file, for the error bodies the matrix compares unnormalised.
func goldenBytes(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testkit.Testdata(t, "golden"), name))
	if err != nil {
		t.Fatalf("golden %s: %v", name, err)
	}
	return data
}

// exerciseReadMatrix runs the authentication, origin and Host matrix over every served GET endpoint,
// then the not-found answers. It fails the test on the first mismatch.
func exerciseReadMatrix(t *testing.T, f *Fixture) {
	t.Helper()
	unauthorized := goldenBytes(t, "error-unauthorized.json")
	forbidden := goldenBytes(t, "error-forbidden.json")
	for _, endpoint := range servedGetEndpoints() {
		path := RequestPath(f, endpoint)
		for _, reply := range []Reply{
			f.Do(Request{Method: "GET", Path: path}),
			f.Do(Request{Method: "GET", Path: path, Token: invalidToken}),
			f.Do(Request{Method: "GET", Path: path, Header: http.Header{"Cookie": {"beltline_auth=" + invalidToken}}}),
		} {
			if reply.Status != 401 || !bytesEqual(reply.Body, unauthorized) {
				t.Fatalf("unauthorized %s: status %d, body %s", endpoint.Case, reply.Status, reply.Body)
			}
		}
		for _, header := range []http.Header{
			{"Origin": {"http://127.0.0.1:" + strconv.Itoa(ContractPort)}},
			{"Cookie": {"beltline_auth=" + ContractToken}, "Origin": {"http://127.0.0.1:" + strconv.Itoa(ContractPort)}},
		} {
			reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken, Header: header})
			if reply.Status != 200 {
				t.Fatalf("authorized %s: status %d", endpoint.Case, reply.Status)
			}
		}
		foreign := []Request{
			{Method: "GET", Path: path, Token: ContractToken, Header: http.Header{"Origin": {"https://foreign.invalid"}}},
			{Method: "GET", Path: path, Header: http.Header{"Origin": {"https://foreign.invalid"}}},
		}
		for _, req := range foreign {
			reply := f.Do(req)
			if reply.Status != 403 || !bytesEqual(reply.Body, forbidden) {
				t.Fatalf("foreign origin %s: status %d", endpoint.Case, reply.Status)
			}
		}
		wrongHost := f.DoRaw(t, "GET "+path+" HTTP/1.1\r\nHost: localhost:"+strconv.Itoa(ContractPort)+
			"\r\nAuthorization: Bearer "+ContractToken+"\r\nConnection: close\r\n\r\n")
		if wrongHost.Status != 403 || !bytesEqual(wrongHost.Body, forbidden) {
			t.Fatalf("wrong host %s: status %d", endpoint.Case, wrongHost.Status)
		}
		missingHost := f.DoRaw(t, "GET "+path+" HTTP/1.1\r\nAuthorization: Bearer "+ContractToken+"\r\nConnection: close\r\n\r\n")
		if missingHost.Status != 403 || !bytesEqual(missingHost.Body, forbidden) {
			t.Fatalf("missing host %s: status %d", endpoint.Case, missingHost.Status)
		}
	}

	notFound := goldenBytes(t, "error-not-found.json")
	for _, path := range []string{"/api/v1/views/run/not-a-run"} {
		reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken})
		if reply.Status != 404 || !bytesEqual(reply.Body, notFound) {
			t.Fatalf("%s: status %d, want the not-found golden", path, reply.Status)
		}
		if f.Do(Request{Method: "GET", Path: path}).Status != 401 {
			t.Fatalf("%s: no token must be 401", path)
		}
		if f.Do(Request{Method: "GET", Path: path, Header: http.Header{"Origin": {"https://foreign.invalid"}}, Token: ContractToken}).Status != 403 {
			t.Fatalf("%s: a foreign origin must be 403", path)
		}
	}
}

// TestAllReadsAreAuthenticatedAndCheckHostAndOriginBeforeLookup checks that every read is authenticated and checks Host and Origin before lookup.
func TestAllReadsAreAuthenticatedAndCheckHostAndOriginBeforeLookup(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	f.SeedViews()
	f.SyncLibrary(t)
	exerciseReadMatrix(t, f)
}

// bytesEqual reports whether a and b are the same bytes.
func bytesEqual(a, b []byte) bool { return bytes.Equal(a, b) }
