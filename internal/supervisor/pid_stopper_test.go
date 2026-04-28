package supervisor

import (
	"errors"
	"os"
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
