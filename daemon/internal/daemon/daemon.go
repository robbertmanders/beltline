// The daemon's lifecycle. Start takes the data folder's lock through the launch session, builds the
// coordinator, starts the listener, publishes the discovery record and says it is ready. Run ticks the
// coordinator's lanes until SIGINT or SIGTERM, then Shutdown tears everything down.

package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/kit/launch"
	"github.com/robbertmanders/beltline/daemon/internal/server"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// buildVersion is stamped by the build: `make` passes it through -ldflags from
// `git describe`. It is "dev" otherwise.
var buildVersion = "dev"

// Mode is the coordinator the daemon runs: live over the data folder, or demo over a fresh one.
type Mode int

const (
	// ModeLive runs over the live data folder.
	ModeLive Mode = iota
	// ModeDemo runs over a demo folder.
	ModeDemo
)

// Hooks are the daemon's seams. Tests replace them; the executable sets the
// notifier and the desktop notifier.
type Hooks struct {
	// TokenSource makes the listener's token.
	TokenSource func() (string, error)
	// ServerFactory builds the listener for a coordinator, token and version.
	ServerFactory func(c *coordinator.Coordinator, token, version string) *server.BeltlineServer
	// Publish writes the discovery record into the data folder.
	Publish func(info launch.DaemonInfo, root string) error
	// ListenerReady is called with the bound port once the listener serves.
	ListenerReady func(port int)
	// ReadyLine is called once the discovery record is published.
	ReadyLine func(port int)
	// OpenDaemon opens a browser on the daemon published in root.
	OpenDaemon func(root string) error
	// OpenFailed receives the browser failure's message; the daemon keeps running.
	OpenFailed func(message string)
	// Notifier observes every relayed notification (tests); nil for none.
	Notifier NotificationObserver
	// DesktopNotifier posts a macOS notification for each one; nil in tests.
	DesktopNotifier NotificationObserver
	// TickInterval is the wait between ticks.
	TickInterval time.Duration
	// TickSleep waits between ticks; nil means the real clock.
	TickSleep TickSleep
}

// NewHooks returns the production hooks, with no-op observers.
// Lock: none.
func NewHooks() Hooks {
	return Hooks{
		TokenSource: launch.DaemonInfoMakeToken,
		ServerFactory: func(c *coordinator.Coordinator, token, version string) *server.BeltlineServer {
			return server.NewBeltlineServer(token, version, c, 0, "", routes.Dependencies{
				DesktopCommandRunner: shell.ProcessRunner{},
				ApplicationLookup:    DesktopApplicationInstalled,
			})
		},
		Publish:       func(info launch.DaemonInfo, root string) error { return info.Publish(root) },
		ListenerReady: func(int) {},
		ReadyLine:     func(int) {},
		OpenDaemon: func(root string) error {
			return DaemonOpen{Runner: shell.ProcessRunner{}, Discovery: newDaemonDiscovery()}.Run(context.Background(), root)
		},
		OpenFailed:   func(string) {},
		TickInterval: DefaultTickInterval,
	}
}

// Daemon is one beltlined process over one data folder.
type Daemon struct {
	mode           Mode
	openAfterStart bool
	livePaths      *store.Paths
	hooks          Hooks
	session        *launch.LaunchSession
	server         *server.BeltlineServer
	ticker         *tickerState
	logger         *DaemonLog
	relay          *NotificationRelay
	discovery      *launch.DaemonInfo
	closed         bool
}

// tickerState is the tick loops' stop signal and their completion.
type tickerState struct {
	stop     chan struct{}
	finished chan struct{}
}

// NewDaemon makes a daemon that has not started. livePaths nil
// means the standard data folder, used only in live mode.
// Lock: none.
func NewDaemon(mode Mode, livePaths *store.Paths, openAfterStart bool, hooks Hooks) *Daemon {
	return &Daemon{mode: mode, livePaths: livePaths, openAfterStart: openAfterStart, hooks: hooks}
}

// LaunchedCoordinator is the coordinator this daemon's launch built, nil before Start.
// Lock: none.
func (d *Daemon) LaunchedCoordinator() *coordinator.Coordinator {
	if d.session == nil {
		return nil
	}
	return d.session.Coordinator
}

// CheckCommand is the command the assistant runs to check its changes: the absolute
// path of the executable and the library-check flag.
// Lock: none (pure).
func CheckCommand(executable string) []string {
	path := executable
	if abs, err := filepath.Abs(executable); err == nil {
		path = abs
	}
	return []string{path, launch.LibraryCheckCommandFlag}
}

// Run starts the daemon, opens a browser when asked, ticks until SIGINT or SIGTERM,
// and shuts down. A start-up failure shuts down and is returned.
// Lock: none; the coordinator takes its own lock for each call.
func (d *Daemon) Run() error {
	ctx := context.Background()
	signals := newTerminationSignals()
	defer signals.Close()
	if err := d.Start(ctx); err != nil {
		d.Shutdown(ctx)
		return err
	}
	d.openBrowserIfRequested()
	if signals.HasReceivedSignal() {
		d.Shutdown(ctx)
		return nil
	}
	d.startTicker()
	signals.Wait()
	d.Shutdown(ctx)
	return nil
}

// startTicker runs one tick loop per lane until Shutdown stops them. A first sessions
// pass recovers what a restart left before the other lanes begin.
// Lock: none; the goroutines' ticks take the coordinator's lock.
func (d *Daemon) startTicker() {
	c := d.session.Coordinator
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		c.TickLane(context.Background(), coordinator.LaneSessions)
		wakes := c.StartLanes()
		var loops sync.WaitGroup
		for _, lane := range coordinator.AllLanes {
			loop := TickLoop{
				Interval: d.hooks.TickInterval,
				Tick: timedTick(func() { c.TickLane(context.Background(), lane) }, time.Now, func(took time.Duration) {
					d.logger.Append("tick lane " + string(lane) + " took " + took.Round(100*time.Millisecond).String())
				}),
				Sleep: d.hooks.TickSleep,
				Wake:  wakes[lane],
			}
			loops.Add(1)
			go func() {
				defer loops.Done()
				loop.Run(stop)
			}()
		}
		loops.Wait()
	}()
	d.ticker = &tickerState{stop: stop, finished: finished}
}

// Start takes the data folder's lock, builds the coordinator and starts the
// listener; on success the discovery record names it and the ready line is printed
// . A failure leaves what it made for Shutdown to release.
// Lock: none. Suspends: the listener's start.
func (d *Daemon) Start(ctx context.Context) error {
	checkCommand := CheckCommand(CurrentExecutable())
	relay := newNotificationRelay()
	d.relay = relay
	relay.SetObserver(d.hooks.Notifier)
	relay.SetDesktop(d.hooks.DesktopNotifier)
	notifier := func(title, body string) { relay.Publish(title, body) }

	var (
		session *launch.LaunchSession
		err     error
	)
	switch d.mode {
	case ModeLive:
		paths := core.ValueOr(d.livePaths, store.StandardPaths(os.Getenv, supportFolderOrTemp()))
		session, err = launch.LaunchSessionLive(paths, notifier, checkCommand, nil)
	case ModeDemo:
		session, err = launch.LaunchSessionDemo(nil, "", notifier, checkCommand, nil)
	}
	if err != nil {
		return describeLaunchFailure(err, d.livePaths)
	}
	d.session = session
	// A quit request stops this process the same way SIGTERM does.
	session.Coordinator.SetQuitHandler(func() { _ = syscall.Kill(os.Getpid(), syscall.SIGTERM) })

	// With the kernel lock held, a discovery record can only be stale. Remove it
	// before any later startup failure can leave it advertising a listener that
	// does not exist.
	launch.DaemonInfoRemove(session.Root)
	logger, err := OpenDaemonLog(session.Root)
	if err != nil {
		session.Close()
		d.session = nil
		return DaemonFailure{Kind: DaemonFailureLog, Folder: session.Root, Message: err.Error()}
	}
	d.logger = logger
	relay.SetLog(logger)

	logger.Append(fmt.Sprintf("Daemon starting (pid %d)", os.Getpid()))
	if message := upgradeMessage(session.Upgrade, session.UpgradeError); message != "" {
		logger.Append(message)
	}
	token, err := d.hooks.TokenSource()
	if err != nil {
		return d.startupFailure(session.Root, err)
	}
	version := BuildVersion()
	newServer := d.hooks.ServerFactory(session.Coordinator, token, version)
	d.server = newServer
	port, err := newServer.Start(ctx)
	if err != nil {
		return d.startupFailure(session.Root, err)
	}
	relay.Connect(newServer)
	d.hooks.ListenerReady(port)
	info := launch.DaemonInfo{
		Pid:        int32(os.Getpid()),
		Port:       port,
		Token:      token,
		Version:    version,
		APIVersion: launch.VersionCurrent,
	}
	if err := d.hooks.Publish(info, session.Root); err != nil {
		return d.startupFailure(session.Root, err)
	}
	d.discovery = &info
	logger.Append("Daemon ready on 127.0.0.1:" + strconv.Itoa(port))
	d.hooks.ReadyLine(port)
	return nil
}

// startupFailure is the error for a start-up step that failed in folder.
// Lock: none (pure).
func (d *Daemon) startupFailure(folder string, err error) error {
	return DaemonFailure{Kind: DaemonFailureStartup, Folder: folder, Message: err.Error()}
}

// openBrowserIfRequested opens the browser on the session just published. A browser
// failure is reported separately and leaves the daemon running.
// Lock: none. Suspends: the open hook.
func (d *Daemon) openBrowserIfRequested() {
	if !d.openAfterStart || d.session == nil {
		return
	}
	if err := d.hooks.OpenDaemon(d.session.Root); err != nil {
		d.hooks.OpenFailed(err.Error())
	}
}

// Shutdown stops the tick loop and the listener, removes the discovery record this
// process published, closes the log and releases the folder.
// It runs once; later calls do nothing.
// Lock: none. Suspends: the tick loop's finish and the listener's stop.
func (d *Daemon) Shutdown(ctx context.Context) {
	if d.closed {
		return
	}
	d.closed = true
	if d.ticker != nil {
		close(d.ticker.stop)
		<-d.ticker.finished
	}
	if d.server != nil {
		d.server.Stop(ctx)
	}
	if d.relay != nil {
		d.relay.Disconnect()
	}
	if d.session != nil && d.discovery != nil {
		d.discovery.Remove(d.session.Root)
	}
	d.logger.Append("Daemon stopped")
	d.logger.Close()
	if d.session != nil {
		d.session.Close()
	}
	d.logger = nil
	d.session = nil
	d.server = nil
	d.ticker = nil
	d.discovery = nil
	d.relay = nil
}

// BuildVersion is the version the daemon reports: BELTLINE_VERSION when set and not
// empty, else the build's stamp, else "dev".
// Lock: none.
func BuildVersion() string {
	if configured := os.Getenv("BELTLINE_VERSION"); configured != "" {
		return configured
	}
	return buildVersion
}

// supportFolderOrTemp is ~/Library/Application Support, or the temporary folder when the home folder is unknown.
// Lock: none.
func supportFolderOrTemp() string {
	support, err := store.SupportFolder()
	if err != nil {
		return os.TempDir()
	}
	return support
}

// DaemonFailureKind says which failure a DaemonFailure is.
type DaemonFailureKind string

const (
	// DaemonFailureHeld: another launch owns the data folder.
	DaemonFailureHeld DaemonFailureKind = "held"
	// DaemonFailureLog: beltlined.log could not be opened.
	DaemonFailureLog DaemonFailureKind = "log"
	// DaemonFailureStartup: a start-up step failed.
	DaemonFailureStartup DaemonFailureKind = "startup"
)

// DaemonFailure is why the daemon could not start.
// Owner is set for held: the pid the lock records, or "unavailable".
type DaemonFailure struct {
	Kind    DaemonFailureKind
	Folder  string
	Owner   string
	Message string
}

// Error describes the failure.
// Lock: none (pure).
func (f DaemonFailure) Error() string {
	switch f.Kind {
	case DaemonFailureHeld:
		return fmt.Sprintf("Data folder already in use: %s (owner pid: %s)", f.Folder, f.Owner)
	case DaemonFailureLog:
		return fmt.Sprintf("Could not open beltlined.log in %s: %s", f.Folder, f.Message)
	}
	return fmt.Sprintf("Could not start daemon for %s: %s", f.Folder, f.Message)
}

// describeLaunchFailure maps a launch failure to the daemon's failure.
// Lock: none (pure).
func describeLaunchFailure(err error, livePaths *store.Paths) error {
	var failure launch.LaunchFailure
	if errors.As(err, &failure) {
		if failure.Kind == launch.LaunchFailureHeld {
			owner := "unavailable"
			if failure.Owner != nil {
				owner = strconv.Itoa(int(*failure.Owner))
			}
			return DaemonFailure{Kind: DaemonFailureHeld, Folder: failure.Folder, Owner: owner}
		}
		return DaemonFailure{Kind: DaemonFailureStartup, Folder: failure.Folder, Message: failure.Message}
	}
	folder := ""
	if livePaths != nil {
		folder = livePaths.Root
	}
	return DaemonFailure{Kind: DaemonFailureStartup, Folder: folder, Message: err.Error()}
}
