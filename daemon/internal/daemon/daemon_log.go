package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/launch"
)

// DaemonLogFileName is the log's name in the data folder.
const DaemonLogFileName = "beltlined.log"

// DaemonLogRotateBytes is the size past which a start moves beltlined.log aside to
// beltlined.log.1 (replacing the one before) and begins a fresh log, so the log is at most
// about twice this size on disk.
const DaemonLogRotateBytes = 5 << 20

// DaemonLog appends lifecycle and notification lines to beltlined.log, which
// stays open for the daemon's life.
type DaemonLog struct {
	file *os.File
	mu   sync.Mutex
}

// OpenDaemonLog opens folder's beltlined.log for appending, mode 0600, after moving it to
// beltlined.log.1 when it has grown past DaemonLogRotateBytes.
// A failure is a daemonLogError naming the file and the reason.
// Lock: none.
func OpenDaemonLog(folder string) (*DaemonLog, error) {
	file := filepath.Join(folder, DaemonLogFileName)
	if info, err := os.Stat(file); err == nil && info.Size() > DaemonLogRotateBytes {
		// A failed rename only means the log keeps growing until the next start.
		_ = os.Rename(file, file+".1")
	}
	descriptor, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, daemonLogError{File: file, Reason: errnoReason(err)}
	}
	if err := descriptor.Chmod(0o600); err != nil {
		_ = descriptor.Close()
		return nil, daemonLogError{File: file, Reason: errnoReason(err)}
	}
	return &DaemonLog{file: descriptor}, nil
}

// Append writes one line: an ISO 8601 timestamp, then message.
// Lock: the log's own mutex.
func (l *DaemonLog) Append(message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"), message)
	_, _ = l.file.Write([]byte(line))
}

// Notification writes a notification line.
// Lock: the log's own mutex, through Append.
func (l *DaemonLog) Notification(title, body string) {
	l.Append("Notification: " + singleLine(title) + " — " + singleLine(body))
}

// Close closes the log file.
// Lock: the log's own mutex.
func (l *DaemonLog) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.file.Close()
}

// singleLine escapes carriage returns and line feeds so one notification stays on one line.
// Lock: none (pure).
func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\r", `\r`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

// daemonLogError names the log file and why it could not be opened.
type daemonLogError struct {
	File   string
	Reason string
}

// Error is "<file>: <reason>".
// Lock: none (pure).
func (e daemonLogError) Error() string { return e.File + ": " + e.Reason }

// errnoReason is the message a Go file error carries, without the path.
// Lock: none (pure).
func errnoReason(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		return launch.StrerrorText(pe.Err)
	}
	if le, ok := err.(*os.LinkError); ok {
		return launch.StrerrorText(le.Err)
	}
	return launch.StrerrorText(err)
}

// upgradeMessage is the log line about a launch's data folder upgrade, or
// empty when it had nothing to do.
func upgradeMessage(report store.UpgradeReport, err error) string {
	if err != nil {
		return "Could not upgrade the data folder, will try again at the next start: " + err.Error()
	}
	if !report.Ran {
		return ""
	}
	message := fmt.Sprintf("Upgraded the data folder: rewrote %d belt versions, %d belt states and %d runs", report.Belts, report.States, report.Runs)
	if report.Agents > 0 {
		message += fmt.Sprintf(", saved %d agent versions they use", report.Agents)
	}
	if report.Settings {
		message += ", and the settings"
	}
	if report.Backup != "" {
		message += "; the files as they were are in " + report.Backup
	}
	if len(report.Unreadable) > 0 {
		message += "; left unreadable files as they are: " + strings.Join(report.Unreadable, ", ")
	}
	return message
}
