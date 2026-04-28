package supervisor

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestIsProcessAlreadyDone(t *testing.T) {
	if !isProcessAlreadyDone(os.ErrProcessDone) {
		t.Fatal("expected os.ErrProcessDone to be treated as done")
	}
	if !isProcessAlreadyDone(&os.SyscallError{Syscall: "kill", Err: syscall.ESRCH}) {
		t.Fatal("expected ESRCH to be treated as done")
	}
	if isProcessAlreadyDone(errors.New("permission denied")) {
		t.Fatal("unexpected done classification")
	}
}

func TestPIDExists(t *testing.T) {
	if pidExists(0) {
		t.Fatal("PID 0 should not be treated as a managed process")
	}
	if runtime.GOOS == "linux" && !pidExists(os.Getpid()) {
		t.Fatal("expected current process to exist on Linux")
	}
}
