package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/kit/demo"
)

// OwnershipLockFileName is the lock file's name in the data folder.
// The name lives in the demo package, which launch imports and which cannot import launch.
const OwnershipLockFileName = demo.LockFileName

// LaunchFailureKind is the case of a LaunchFailure.
type LaunchFailureKind string

const (
	// LaunchFailureHeld: another launch holds the folder's lock; Owner is the pid it recorded, nil while not readable yet.
	LaunchFailureHeld LaunchFailureKind = "held"
	// LaunchFailureIO: the folder or its lock file could not be created, opened or locked.
	LaunchFailureIO LaunchFailureKind = "io"
	// LaunchFailureNewerFormat: the folder was saved by a newer Beltline (store.NewerFormat).
	LaunchFailureNewerFormat LaunchFailureKind = "newerFormat"
)

// LaunchFailure is why a launch could not own its data folder.
type LaunchFailure struct {
	Kind    LaunchFailureKind
	Folder  string
	Owner   *int32
	Message string
}

// Error describes the failure.
// Lock: none (pure).
func (f LaunchFailure) Error() string {
	if f.Kind == LaunchFailureHeld {
		if f.Owner != nil {
			return fmt.Sprintf("Another launch holds %s (pid %d).", f.Folder, *f.Owner)
		}
		return fmt.Sprintf("Another launch holds %s.", f.Folder)
	}
	return f.Message
}

// OwnershipLock is one launch's hold on a data folder: an exclusive flock on
// `<folder>/daemon.lock`, which records the holder's pid. The kernel lock is the
// authority: a pid left by an owner that quit or died never blocks, and the file
// is never removed or replaced. The descriptor is close-on-exec, so child processes cannot keep the lock
// after the owner exits. Release it with Release; Go has no deinit, so the daemon releases it on shutdown.
type OwnershipLock struct {
	Folder string
	// state guards descriptor.
	state sync.Mutex
	// descriptor is the open lock file; -1 once released.
	descriptor int
}

// OwnershipLockFile is the lock file in folder.
// Lock: none (pure).
func OwnershipLockFile(folder string) string {
	return filepath.Join(folder, OwnershipLockFileName)
}

// OwnershipLockAcquire takes folder, creating it when needed, and records pid in
// its lock file. A folder another launch holds, in this process or another, is
// refused at once with the pid that launch recorded. Once held, the folder is made
// private to its owner (DataFolderMode), whatever mode it was made with before.
// Lock: none; it takes the OS lock on the folder's file.
func OwnershipLockAcquire(folder string, pid int32) (*OwnershipLock, error) {
	if err := os.MkdirAll(folder, DataFolderMode); err != nil {
		return nil, LaunchFailure{Kind: LaunchFailureIO, Folder: folder,
			Message: "Could not create the data folder: " + err.Error()}
	}
	path := OwnershipLockFile(folder)
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC, 0o644)
	if err != nil {
		return nil, LaunchFailure{Kind: LaunchFailureIO, Folder: folder,
			Message: fmt.Sprintf("Could not open %s: %s", path, err.Error())}
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer func() { _ = syscall.Close(fd) }()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, LaunchFailure{Kind: LaunchFailureHeld, Folder: folder, Owner: ownershipLockReadOwner(fd)}
		}
		return nil, LaunchFailure{Kind: LaunchFailureIO, Folder: folder,
			Message: fmt.Sprintf("Could not lock %s: %s", path, err.Error())}
	}
	text := []byte(strconv.Itoa(int(pid)) + "\n")
	if err := syscall.Ftruncate(fd, 0); err != nil {
		return nil, ownershipLockRecordFailure(fd, folder, path, err)
	}
	if n, err := syscall.Pwrite(fd, text, 0); err != nil || n != len(text) {
		if err == nil {
			err = syscall.EIO
		}
		return nil, ownershipLockRecordFailure(fd, folder, path, err)
	}
	_ = syscall.Fsync(fd)
	// Runs hold issue text, agent output and transcripts, and the checkouts hold private code: only the
	// owner may enter the folder, so the modes of what is inside it don't matter to other users.
	if err := os.Chmod(folder, DataFolderMode); err != nil {
		_ = syscall.Close(fd)
		return nil, LaunchFailure{Kind: LaunchFailureIO, Folder: folder,
			Message: fmt.Sprintf("Could not make the data folder private to you (chmod 700): %s", err.Error())}
	}
	return &OwnershipLock{Folder: folder, descriptor: fd}, nil
}

// DataFolderMode is the data folder's mode: readable, writable and enterable by its owner only.
const DataFolderMode os.FileMode = 0o700

// ownershipLockRecordFailure closes the descriptor and reports that the pid could not be recorded.
func ownershipLockRecordFailure(fd int, folder, path string, err error) error {
	_ = syscall.Close(fd)
	return LaunchFailure{Kind: LaunchFailureIO, Folder: folder,
		Message: fmt.Sprintf("Could not record the owner in %s: %s", path, err.Error())}
}

// OwnershipLockRecordedOwner is the pid recorded in folder's lock file, whoever holds it;
// nil when there is none to read.
// Lock: none.
func OwnershipLockRecordedOwner(folder string) *int32 {
	fd, err := syscall.Open(OwnershipLockFile(folder), syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = syscall.Close(fd) }()
	return ownershipLockParse(fd)
}

// Held reports whether this lock still holds its folder.
// Lock: takes the lock's own state mutex.
func (l *OwnershipLock) Held() bool {
	l.state.Lock()
	defer l.state.Unlock()
	return l.descriptor >= 0
}

// Release lets the folder go by closing the lock file, which stays for the next
// owner. Closing, not unlocking, so a descriptor that leaked into a child would
// keep the lock rather than hide the leak.
// Lock: takes the lock's own state mutex.
func (l *OwnershipLock) Release() {
	l.state.Lock()
	defer l.state.Unlock()
	if l.descriptor < 0 {
		return
	}
	_ = syscall.Close(l.descriptor)
	l.descriptor = -1
}

// ownershipLockReadOwner reads the holder's pid, retrying briefly while the holder
// writes it.
// Lock: none.
func ownershipLockReadOwner(fd int) *int32 {
	for attempt := range 20 {
		if pid := ownershipLockParse(fd); pid != nil {
			return pid
		}
		if attempt < 19 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return nil
}

// ownershipLockParse reads the pid text from fd, or nil.
// Lock: none.
func ownershipLockParse(fd int) *int32 {
	buffer := make([]byte, 32)
	count, err := syscall.Pread(fd, buffer, 0)
	if err != nil || count <= 0 {
		return nil
	}
	data := buffer[:count]
	if !utf8.Valid(data) {
		return nil
	}
	text := string(data)
	if !strings.HasSuffix(text, "\n") {
		return nil
	}
	pid, err := strconv.ParseInt(strings.TrimSuffix(text, "\n"), 10, 32)
	if err != nil || pid <= 0 {
		return nil
	}
	value := int32(pid)
	return &value
}
