package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/kit/launch"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// TestReturnsTheCreatorsVersion
func TestReturnsTheCreatorsVersion(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"dev", "1.4.0 (2026.10.06-abc1234)", "0.1.0+build.7"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			h := testRouter(testToken, version, testWebRoot(t), nil)
			reply := send(h, http.MethodGet, "/api/v1/version", bearerHeader(testToken))
			if reply.status != http.StatusOK {
				t.Fatalf("status %d", reply.status)
			}
			if got := reply.header("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type %q", got)
			}
			var object map[string]any
			if err := json.Unmarshal(reply.body, &object); err != nil {
				t.Fatalf("body: %v", err)
			}
			if len(object) != 3 || object["apiVersion"] != float64(1) || object["version"] != version || object["platform"] != "macos" {
				t.Fatalf("body %s", reply.body)
			}
			want, err := routes.JSONEncode(launch.Version{APIVersion: 1, Version: version, Platform: "macos"})
			if err != nil {
				t.Fatal(err)
			}
			if string(reply.body) != string(want) {
				t.Fatalf("body %s, want %s", reply.body, want)
			}
			if !strings.HasPrefix(reply.text(), `{"apiVersion":1,"version":`) {
				t.Fatalf("key order: %s", reply.body)
			}
			if strings.Contains(reply.text(), testToken) || strings.Contains(reply.text(), strconv.Itoa(os.Getpid())) {
				t.Fatalf("body leaks credentials or pid: %s", reply.body)
			}
		})
	}
}

// TestMatchesTheGolden
func TestMatchesTheGolden(t *testing.T) {
	t.Parallel()
	h := testRouter(testToken, "dev", testWebRoot(t), nil)
	reply := send(h, http.MethodGet, "/api/v1/version", bearerHeader(testToken))
	if string(reply.body) != string(golden(t, "version.json")) {
		t.Fatalf("body %s, golden %s", reply.body, golden(t, "version.json"))
	}
}
