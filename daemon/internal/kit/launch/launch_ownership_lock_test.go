package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// wantHeld asserts err is a held LaunchFailure for folder, with owner (nil for none).
func wantHeld(t *testing.T, err error, folder string, owner *int32) {
	t.Helper()
	failure, ok := err.(LaunchFailure)
	if !ok {
		t.Fatalf("error = %v, want a held LaunchFailure", err)
	}
	if failure.Kind != LaunchFailureHeld || failure.Folder != folder {
		t.Fatalf("failure = %+v, want held for %s", failure, folder)
	}
	switch {
	case owner == nil && failure.Owner != nil:
		t.Fatalf("owner = %d, want none", *failure.Owner)
	case owner != nil && (failure.Owner == nil || *failure.Owner != *owner):
		t.Fatalf("owner = %v, want %d", failure.Owner, *owner)
	}
}

func TestASecondAcquisitionIsRefusedWithTheFirstPid(t *testing.T) {
	t.Parallel()
	folder := filepath.Join(t.TempDir(), "data")
	first, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	file := OwnershipLockFile(folder)
	recorded := readText(file)
	if recorded != fmt.Sprintf("%d\n", os.Getpid()) {
		t.Fatalf("recorded = %q", recorded)
	}
	_, err = OwnershipLockAcquire(folder, 1)
	wantHeld(t, err, folder, new(int32(os.Getpid())))
	if got := readText(file); got != recorded {
		t.Errorf("file changed to %q", got)
	}
	if got := lockProbe(t, folder); got != fmt.Sprintf("held %d", os.Getpid()) {
		t.Errorf("probe = %q", got)
	}
	if got := readText(file); got != recorded {
		t.Errorf("file changed by the probe to %q", got)
	}
	first.Release()
}

func TestEquivalentPathsToTheFolderAreTheSameLock(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	folder := filepath.Join(parent, "data")
	link := filepath.Join(parent, "link")
	lock, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := os.Symlink(folder, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err = OwnershipLockAcquire(link, int32(os.Getpid()))
	wantHeld(t, err, link, new(int32(os.Getpid())))
	dotted := parent + "/./link/../data"
	_, err = OwnershipLockAcquire(dotted, int32(os.Getpid()))
	wantHeld(t, err, dotted, new(int32(os.Getpid())))
	lock.Release()
}

func TestAnotherProcessHoldingTheFolderIsReportedAndKept(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	holder := startLockHolder(t, folder)
	if got := holder.line(); got != "ready" {
		t.Fatalf("holder says %q", got)
	}
	_, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	wantHeld(t, err, folder, new(holder.pid))
	if got := readText(OwnershipLockFile(folder)); got != fmt.Sprintf("%d\n", holder.pid) {
		t.Errorf("file = %q", got)
	}
}

func TestAReleasedFolderCanBeTakenAgainWithTheSameFile(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	file := OwnershipLockFile(folder)
	first, err := OwnershipLockAcquire(folder, 4242)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	inode := inodeOf(file)
	first.Release()
	if first.Held() {
		t.Fatal("still held after release")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("lock file removed: %v", err)
	}
	// A fork another parallel test is making may still hold a copy of the released
	// descriptor until its exec, so the folder is taken once that copy is gone.
	var second *OwnershipLock
	testkit.Eventually(t, 5*time.Second, "the released folder can be taken", func() bool {
		lock, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
		second = lock
		return err == nil
	})
	if got := readText(file); got != fmt.Sprintf("%d\n", os.Getpid()) {
		t.Errorf("file = %q", got)
	}
	if inodeOf(file) != inode {
		t.Error("the lock file was replaced")
	}
	second.Release()
}

func TestADeadOwnersFolderCanBeTakenWithoutDeletingItsFile(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	file := OwnershipLockFile(folder)
	holder := startLockHolder(t, folder)
	if got := holder.line(); got != "ready" {
		t.Fatalf("holder says %q", got)
	}
	dead := holder.pid
	inode := inodeOf(file)
	holder.kill()
	// The dead owner's pid is still recorded, and does not block.
	if got := readText(file); got != fmt.Sprintf("%d\n", dead) {
		t.Fatalf("file = %q", got)
	}
	if owner := OwnershipLockRecordedOwner(folder); owner == nil || *owner != dead {
		t.Fatalf("recorded owner = %v, want %d", owner, dead)
	}
	lock, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	if err != nil {
		t.Fatalf("acquire after the owner died: %v", err)
	}
	if got := readText(file); got != fmt.Sprintf("%d\n", os.Getpid()) {
		t.Errorf("file = %q", got)
	}
	if inodeOf(file) != inode {
		t.Error("the lock file was replaced")
	}
	lock.Release()
}

func TestAStalePidNeverPermitsTakingAHeldLock(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	file := OwnershipLockFile(folder)
	holder := startLockHolder(t, folder)
	if got := holder.line(); got != "ready" {
		t.Fatalf("holder says %q", got)
	}
	// Overwrite the recorded pid with one whose process is gone.
	stale := unusedProcessID()
	if err := os.WriteFile(file, []byte(fmt.Sprintf("%d\n", stale)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	wantHeld(t, err, folder, new(stale))
}

func TestAnUnreadablePidIsReportedAsNone(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	holder := startLockHolder(t, folder)
	if got := holder.line(); got != "ready" {
		t.Fatalf("holder says %q", got)
	}
	if err := os.WriteFile(OwnershipLockFile(folder), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	wantHeld(t, err, folder, nil)
}

func TestAFolderThatCannotBeCreatedIsAnIOError(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	blocker := filepath.Join(parent, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	folder := filepath.Join(blocker, "data")
	_, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	failure, ok := err.(LaunchFailure)
	if !ok || failure.Kind != LaunchFailureIO || failure.Folder != folder {
		t.Fatalf("error = %v, want an io LaunchFailure for %s", err, folder)
	}
}

// A child the owner starts must not keep the lock once the owner lets go.
func TestChildProcessesDoNotInheritTheLock(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	lock, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(lock.descriptor), syscall.F_GETFD, 0)
	if errno != 0 || flags&syscall.FD_CLOEXEC == 0 {
		t.Fatalf("descriptor is not close-on-exec (flags %d, errno %v)", flags, errno)
	}
	child := exec.Command("/bin/sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	lock.Release()
	if err := syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatalf("child is not running: %v", err)
	}
	// As above, a parallel fork may still hold the descriptor for a moment.
	testkit.Eventually(t, 5*time.Second, "the probe finds the folder free", func() bool {
		return lockProbe(t, folder) == "free"
	})
}

func TestTheDataFolderIsPrivateToItsOwner(t *testing.T) {
	t.Parallel()
	fresh := filepath.Join(t.TempDir(), "fresh")
	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{fresh, shared} {
		lock, err := OwnershipLockAcquire(folder, int32(os.Getpid()))
		if err != nil {
			t.Fatalf("acquire %s: %v", folder, err)
		}
		lock.Release()
		info, err := os.Stat(folder)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("%s mode = %04o, want 0700", folder, got)
		}
	}
}
