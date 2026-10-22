//go:build integration

// Socket tests of the event stream: the read endpoints after hello, the unread peer, the stop during a
// stalled write, and the unchanged idle tick. They bind a real loopback port through BeltlineServer,
// so they sit behind the integration tag: run them with go test -tags integration -count=1
// ./internal/server/.

package server

import (
	"bufio"
	"context"
	"fmt"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// eventsTestToken is the creator's token the socket tests sign in with.
const eventsTestToken = "events-test-token-0123456789abcdef"

// eventsTestVersion is the daemon version the socket tests serve.
const eventsTestVersion = "test"

// eventsServer is a running BeltlineServer over a demo coordinator (the
// ContractFixture for the socket tests).
type eventsServer struct {
	c      *coordinator.Coordinator
	server *BeltlineServer
	port   int
}

// startEventsServer starts a server on a free port over a fresh demo coordinator.
// eventWrite configures its socket writes (production values when zero).
func startEventsServer(t *testing.T, eventWrite routes.EventWriteConfiguration) *eventsServer {
	t.Helper()
	c := newEventsCoordinator(t, nil)
	if eventWrite.Deadline == 0 {
		eventWrite = routes.EventWriteConfigurationProduction
	}
	server := newBeltlineServerForTest(eventsTestToken, eventsTestVersion, DefaultPort, "127.0.0.1", t.TempDir(), c,
		eventWrite, []byte("events-test-signing-key-0123456789"), time.Now, routes.Dependencies{}, nil)
	port, err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { server.Stop(context.Background()) })
	return &eventsServer{c: c, server: server, port: port}
}

// eventsURL is the events socket's URL on port.
func eventsURL(port int) string {
	return fmt.Sprintf("ws://127.0.0.1:%d/api/v1/events", port)
}

// dialEvents opens the events socket with the given headers.
func dialEvents(t *testing.T, port int, headers http.Header) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), eventWait)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, eventsURL(port), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// The demo's state frames run past the default 32 KiB read limit.
	conn.SetReadLimit(16 << 20)
	t.Cleanup(func() { _ = conn.CloseNow() }) // the test is done with this connection; a close error has nothing to report
	return conn
}

// eventsBearer is the Authorization and Origin headers of a signed-in socket.
func eventsBearer(port int) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+eventsTestToken)
	h.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", port))
	return h
}

// readFrame reads the next text frame, failing the test when none comes in time.
func readFrame(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), eventWait)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return string(data)
}

func TestBearerSocketStreamsHelloChangesAndNotificationsAsCanonicalText(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	conn := dialEvents(t, fx.port, eventsBearer(fx.port))

	if got := readFrame(t, conn); got != readEventGolden(t, "event-hello.json") {
		t.Fatalf("hello = %s", got)
	}
	fx.server.PublishNotification("Needs attention", "Check the build")
	if got := readFrame(t, conn); got != readEventGolden(t, "event-notification.json") {
		t.Fatalf("notification = %s", got)
	}

	// Changing a run: runs, then state, in one turn.
	first := fx.c.Runs()[0]
	if !fx.c.UpdateRunForContract(first.ID, func(run *core.Run) { run.Subject.Title += " changed" }) {
		t.Fatalf("the run did not change")
	}
	if got := readFrame(t, conn); got != readEventGolden(t, "event-changed.json") {
		t.Fatalf("changed = %s", got)
	}
	if got := readFrame(t, conn); got != changedFrame("state", 2) {
		t.Fatalf("state = %s", got)
	}

	belt := ""
	for _, record := range fx.c.Belts() {
		if !record.Paused {
			belt = record.Belt.ID
			break
		}
	}

	// Pausing the belt: library, then the login: settings and state, then the global
	// pause: settings and state again.
	if err := fx.c.SetPaused(belt, true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	if got := readFrame(t, conn); got != changedFrame("library", 3) {
		t.Fatalf("library = %s", got)
	}
	mustSetLogin(t, fx.c, "event-client")
	if got := readFrame(t, conn); got != changedFrame("settings", 4) {
		t.Fatalf("settings = %s", got)
	}
	if got := readFrame(t, conn); got != changedFrame("state", 5) {
		t.Fatalf("state = %s", got)
	}
	fx.c.SetGloballyPaused(context.Background(), true)
	requireFrameResources(t, conn, "settings", "state")
}

func TestCookieFromAuthRouteCanOpenSocketAndNotificationsPreserveText(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	reply, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/auth?token=%s", fx.port, eventsTestToken))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	_ = reply.Body.Close() // the response is read; a close error has nothing left to report
	if reply.StatusCode != http.StatusFound {
		t.Fatalf("auth status = %d", reply.StatusCode)
	}
	cookie := strings.SplitN(reply.Header.Get("Set-Cookie"), ";", 2)[0]

	headers := http.Header{}
	headers.Set("Cookie", cookie)
	headers.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", fx.port))
	conn := dialEvents(t, fx.port, headers)
	if got := readFrame(t, conn); got != readEventGolden(t, "event-hello.json") {
		t.Fatalf("hello = %s", got)
	}
	fx.server.PublishNotification("Build ⚙️", "First line\nsecond line\r\n✓")
	got := readFrame(t, conn)
	if !strings.Contains(got, `\n`) || !strings.Contains(got, `\r`) {
		t.Fatalf("notification escaping lost: %s", got)
	}
}

func TestStopClosesSocketsAndRestartStartsFreshObservationAndRevisionLifetime(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	first := dialEvents(t, fx.port, eventsBearer(fx.port))
	healthy := dialEvents(t, fx.port, eventsBearer(fx.port))
	hello := readEventGolden(t, "event-hello.json")
	if got := readFrame(t, first); got != hello {
		t.Fatalf("first hello = %s", got)
	}
	if got := readFrame(t, healthy); got != hello {
		t.Fatalf("healthy hello = %s", got)
	}
	if state := fx.server.eventDebugState(); state == nil || state.Clients != 2 || state.Writers != 2 {
		t.Fatalf("started = %+v", state)
	}

	_ = first.CloseNow() // the test is done with this connection; a close error has nothing to report
	testkit.Eventually(t, eventWait, "one client left", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 1 && state.Writers == 1
	})
	// The coordinator keeps going with the closed client: a library change still reaches the other.
	fx.c.SetBeltArchived(firstUnarchivedBeltID(t, fx.c), true)
	requireFrameResources(t, healthy, "library")

	fx.server.Stop(context.Background())
	if state := fx.server.eventDebugState(); state != nil {
		t.Fatalf("stopped server still reports %+v", state)
	}
	port, err := fx.server.Start(context.Background())
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	restarted := fx.server.eventDebugState()
	if restarted == nil || restarted.Clients != 0 || restarted.Writers != 0 {
		t.Fatalf("restarted = %+v", restarted)
	}
	second := dialEvents(t, port, eventsBearer(port))
	if got := readFrame(t, second); got != hello {
		t.Fatalf("second hello = %s", got)
	}
	fx.server.Stop(context.Background())
}

// stallNotifications returns a write configuration that holds every notification
// write for stall, and a channel that receives when the first one is held.
func stallNotifications(deadline, stall time.Duration) (routes.EventWriteConfiguration, <-chan struct{}) {
	started := make(chan struct{}, 1)
	cfg := routes.EventWriteConfiguration{
		Deadline:      deadline,
		StallDuration: stall,
		ShouldStall: func(message string) bool {
			if !strings.Contains(message, `"type":"notification"`) {
				return false
			}
			select {
			case started <- struct{}{}:
			default:
			}
			return true
		},
	}
	return cfg, started
}

// requireSocketClosed reads frames until the connection reports an error: the server
// closed it.
func requireSocketClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), eventWait)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func TestStopClosesSocketAndFinishesAStalledNetworkWritePromptly(t *testing.T) {
	cfg, stalled := stallNotifications(30*time.Second, 30*time.Second)
	fx := startEventsServer(t, cfg)
	conn := dialEvents(t, fx.port, eventsBearer(fx.port))
	testkit.Eventually(t, eventWait, "the client is registered", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 1 && state.Writers == 1
	})
	fx.server.PublishNotification("stall", "hold this outbound write")
	requireSignal(t, stalled, "the stalled write starts")
	testkit.Eventually(t, eventWait, "the write is in flight", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Writes == 1
	})

	began := time.Now()
	fx.server.Stop(context.Background())
	if elapsed := time.Since(began); elapsed >= 5*time.Second {
		t.Fatalf("stop waited %v for the stalled write", elapsed)
	}
	requireSocketClosed(t, conn)

	port, err := fx.server.Start(context.Background())
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if port != fx.port {
		t.Fatalf("restarted on %d, want %d", port, fx.port)
	}
	if state := fx.server.eventDebugState(); state == nil || state.Clients != 0 || state.Writers != 0 || state.Writes != 0 {
		t.Fatalf("restarted = %+v", state)
	}
}

func TestStalledWriteDeadlineClosesTheUnderlyingSocket(t *testing.T) {
	cfg, stalled := stallNotifications(5*time.Second, 30*time.Second)
	fx := startEventsServer(t, cfg)
	conn := dialEvents(t, fx.port, eventsBearer(fx.port))
	testkit.Eventually(t, eventWait, "the client is registered", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 1 && state.Writers == 1
	})
	fx.server.PublishNotification("deadline", "hold until deadline")
	requireSignal(t, stalled, "the stalled write starts")

	testkit.Eventually(t, 2*eventWait, "the deadline drops the client", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 0 && state.Writers == 0 && state.Writes == 0
	})
	requireSocketClosed(t, conn)
}

func TestMissingWrongAndQueryOnlyCredentialsReturnUnauthorizedGolden(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	base := rawEventsHeaders(fx.port)
	golden := readEventGolden(t, "error-unauthorized.json")
	requireRawRejection(t, fx.port, "/api/v1/events", base, http.StatusUnauthorized, golden)
	requireRawRejection(t, fx.port, "/api/v1/events", append(base, "Authorization: Bearer wrong"), http.StatusUnauthorized, golden)
	requireRawRejection(t, fx.port, "/api/v1/events?token="+eventsTestToken, base, http.StatusUnauthorized, golden)
}

func TestInvalidAndDuplicateHostOrOriginReturnForbiddenBeforeAuthentication(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	port := fx.port
	base := rawEventsHeaders(port)
	golden := readEventGolden(t, "error-forbidden.json")
	// net/http answers 400 itself for no Host or two Host headers, before any
	// handler runs (a recorded deviation from the 403 the contract gives).
	for _, headers := range [][]string{
		base[1:],
		append(append([]string{}, base...), fmt.Sprintf("Host: 127.0.0.1:%d", port)),
	} {
		requireRawStatus(t, port, "/api/v1/events", headers, http.StatusBadRequest)
	}
	bad := [][]string{
		replaceHeader(base, "Host:", "Host: localhost:"+fmt.Sprint(port)),
		append(append([]string{}, base...), "Origin: https://foreign.example"),
		append(append([]string{}, base...), "Origin: null"),
		append(append([]string{}, base...), fmt.Sprintf("Origin: http://127.0.0.1:%d", port), fmt.Sprintf("Origin: http://127.0.0.1:%d", port)),
	}
	for _, headers := range bad {
		requireRawRejection(t, port, "/api/v1/events", headers, http.StatusForbidden, golden)
	}
}

func TestConflictingOrDuplicateCredentialsReturnUnauthorizedGolden(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	base := rawEventsHeaders(fx.port)
	golden := readEventGolden(t, "error-unauthorized.json")
	creds := [][]string{
		{"Authorization: Bearer wrong", "Cookie: beltline_auth=" + eventsTestToken},
		{"Authorization: Bearer " + eventsTestToken, "Authorization: Bearer " + eventsTestToken},
		{"Cookie: beltline_auth=" + eventsTestToken + "; beltline_auth=wrong"},
	}
	for _, extra := range creds {
		requireRawRejection(t, fx.port, "/api/v1/events", append(append([]string{}, base...), extra...), http.StatusUnauthorized, golden)
	}
}

func TestExactHostAndAbsentOriginCanUpgrade(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+eventsTestToken)
	conn := dialEvents(t, fx.port, headers)
	if got := readFrame(t, conn); got != readEventGolden(t, "event-hello.json") {
		t.Fatalf("hello = %s", got)
	}
}

// rawEventsHeaders are the handshake headers of a valid upgrade, the Host exact.
func rawEventsHeaders(port int) []string {
	return []string{
		fmt.Sprintf("Host: 127.0.0.1:%d", port),
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
		"Sec-WebSocket-Version: 13",
	}
}

// replaceHeader returns headers with the first one starting with prefix replaced.
func replaceHeader(headers []string, prefix, value string) []string {
	out := append([]string{}, headers...)
	for i, h := range out {
		if strings.HasPrefix(h, prefix) {
			out[i] = value
		}
	}
	return out
}

// requireRawRejection sends one raw handshake with headers and checks the status
// and the exact body. A raw request keeps duplicate and missing headers as sent.
func requireRawRejection(t *testing.T, port int, path string, headers []string, status int, golden string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), eventWait)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }() // the test is done with this connection; a close error has nothing to report
	request := strings.Join(append(append([]string{"GET " + path + " HTTP/1.1"}, headers...), "", ""), "\r\n")
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(eventWait)) // a deadline on a test connection; a failure shows up at the next read
	reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	body, _ := io.ReadAll(reply.Body)
	if reply.StatusCode != status || string(body) != golden {
		t.Fatalf("status %d body %s; want %d %s", reply.StatusCode, body, status, golden)
	}
}

// requireRawStatus sends a raw request and checks only the status.
func requireRawStatus(t *testing.T, port int, path string, headers []string, status int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), eventWait)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }() // the test is done with this connection; a close error has nothing to report
	request := strings.Join(append(append([]string{"GET " + path + " HTTP/1.1"}, headers...), "", ""), "\r\n")
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(eventWait)) // a deadline on a test connection; a failure shows up at the next read
	reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if reply.StatusCode != status {
		t.Fatalf("status %d; want %d", reply.StatusCode, status)
	}
}

func TestUnreadPeerIsClosedAtCapacityWhileHealthyClientAndTickContinue(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	healthy := dialEvents(t, fx.port, eventsBearer(fx.port))
	readFrame(t, healthy) // hello

	// An unread peer: a raw upgrade whose socket is never read.
	slow, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", fx.port))
	if err != nil {
		t.Fatalf("slow dial: %v", err)
	}
	defer func() { _ = slow.Close() }() // the test is done with this connection; a close error has nothing to report
	raw := strings.Join(append(append(rawEventsHeaders(fx.port), "Authorization: Bearer "+eventsTestToken,
		fmt.Sprintf("Origin: http://127.0.0.1:%d", fx.port)), "", ""), "\r\n")
	_, _ = io.WriteString(slow, "GET /api/v1/events HTTP/1.1\r\n"+raw) // the test writes a raw request; a short write fails the reply check
	testkit.Eventually(t, eventWait, "the unread peer is registered", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 2
	})

	body := strings.Repeat("x", eventMaximumMessageBytes/4-128)
	for i := 0; i < 80; i++ {
		fx.server.PublishNotification(fmt.Sprintf("bulk-%d", i), body)
		readFrame(t, healthy)
	}
	testkit.Eventually(t, eventWait, "the unread peer is dropped", func() bool {
		state := fx.server.eventDebugState()
		return state != nil && state.Clients == 1
	})
}

func TestOrdinaryReadRoutesRemainAvailableBesideWebSocketUpgrade(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	conn := dialEvents(t, fx.port, eventsBearer(fx.port))
	readFrame(t, conn) // hello

	reply := eventsGet(t, fx.port, "/api/v1/runs")
	if reply.StatusCode != http.StatusOK || reply.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("runs = %d %s", reply.StatusCode, reply.Header.Get("Content-Type"))
	}
}

func TestReadEndpointsRemainRefetchableAfterHelloAndRelevantChangesWhilePaused(t *testing.T) {
	fx := startEventsServer(t, routes.EventWriteConfiguration{})
	conn := dialEvents(t, fx.port, eventsBearer(fx.port))
	readFrame(t, conn) // hello
	for _, endpoint := range []string{"runs", "belts", "agents", "settings", "state"} {
		if reply := eventsGet(t, fx.port, "/api/v1/"+endpoint); reply.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", endpoint, reply.StatusCode)
		}
	}

	// A changed run: runs and state are refetched.
	first := fx.c.Runs()[0]
	if !fx.c.UpdateRunForContract(first.ID, func(run *core.Run) { run.Subject.Title += " refetch" }) {
		t.Fatalf("the run did not change")
	}
	requireFrameResources(t, conn, "runs", "state")
	if body := eventsBody(t, fx.port, "/api/v1/runs"); !strings.Contains(body, "refetch") {
		t.Fatalf("runs after refetch lack the change")
	}
	eventsBody(t, fx.port, "/api/v1/state")

	// An archived belt: library.
	belt := firstUnarchivedBeltID(t, fx.c)
	fx.c.SetBeltArchived(belt, true)
	requireFrameResources(t, conn, "library")
	for _, endpoint := range []string{"belts", "agents", "belts/" + belt} {
		eventsBody(t, fx.port, "/api/v1/"+endpoint)
	}

	// A saved setting: settings and state.
	mustSetLogin(t, fx.c, "refetch settings")
	requireFrameResources(t, conn, "settings", "state")
	if body := eventsBody(t, fx.port, "/api/v1/settings"); !strings.Contains(body, "refetch settings") {
		t.Fatalf("settings after refetch lack the login")
	}

	// Pausing: settings and state. (The book.note step has no exported seam; the notice
	// path is covered by the coordinator's projection tests.)
	fx.c.SetGloballyPaused(context.Background(), true)
	requireFrameResources(t, conn, "settings", "state")
	eventsBody(t, fx.port, "/api/v1/state")

	// A paused coordinator still emits observed changes between ticks.
	paused := fx.c.Runs()[0]
	if !fx.c.UpdateRunForContract(paused.ID, func(run *core.Run) { run.Subject.Body += " changed while paused" }) {
		t.Fatalf("the paused run did not change")
	}
	requireFrameResources(t, conn, "runs", "state")
	other := ""
	for _, record := range fx.c.Belts() {
		if !record.Archived && record.Belt.ID != belt {
			other = record.Belt.ID
			break
		}
	}
	fx.c.SetBeltArchived(other, true)
	requireFrameResources(t, conn, "library")
}

// eventsBody reads path with the bearer token and fails unless it answers 200.
func eventsBody(t *testing.T, port int, path string) string {
	t.Helper()
	reply := eventsGet(t, port, path)
	body, _ := io.ReadAll(reply.Body)
	if reply.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d", path, reply.StatusCode)
	}
	return string(body)
}

// requireFrameResources reads changed frames until every wanted resource has come,
// as requireResources does (frames for other resources are skipped).
func requireFrameResources(t *testing.T, conn *websocket.Conn, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	for len(seen) < len(want) {
		frame := readFrame(t, conn)
		for _, name := range want {
			if strings.Contains(frame, `"resource":"`+name+`"`) {
				seen[name] = true
			}
		}
	}
}

// eventsGet is an authorised GET of path on port.
func eventsGet(t *testing.T, port int, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+eventsTestToken)
	reply, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	t.Cleanup(func() { _ = reply.Body.Close() }) // the response is read; a close error has nothing left to report
	return reply
}

// TestUnchangedIdleTickEmitsNothing is in this file, not with the unit tests: the
// demo's tick polls its sources and its runs, so it is slow (seconds without the
// race detector, over a minute with it). The contract fixture is empty, which
// is what makes its tick instant; this runs the demo with its belts archived.
func TestUnchangedIdleTickEmitsNothing(t *testing.T) {
	t.Parallel()
	c := newEventsCoordinator(t, nil)
	for _, record := range c.Belts() {
		c.SetBeltArchived(record.Belt.ID, true)
	}
	hub := NewEventHub(c)
	id, client, ok := hub.connect()
	if !ok {
		t.Fatalf("connect refused")
	}
	defer func() {
		hub.Disconnect(id)
		hub.Stop()
	}()
	requireEventMessages(t, client, helloMessage)

	// The first tick settles the fixture (its last check); the next one changes nothing. Idle waits
	// for the work each tick spawns and the hub's flush, so a change the first tick started can't
	// arrive after the drain and look like the second tick's.
	c.Tick(context.Background())
	c.Idle()
	for {
		if _, ok := client.TakePendingForTesting(); !ok {
			break
		}
	}
	c.Tick(context.Background())
	c.Idle()
	if data, ok := client.TakePendingForTesting(); ok {
		t.Fatalf("an unchanged idle tick emitted %s", data)
	}
}

// firstUnarchivedBeltID is the id of the first belt not archived.
func firstUnarchivedBeltID(t *testing.T, c *coordinator.Coordinator) string {
	t.Helper()
	for _, record := range c.Belts() {
		if !record.Archived {
			return record.Belt.ID
		}
	}
	t.Fatalf("the fixture has no unarchived belt")
	return ""
}
