//go:build linux

package daemon

import (
	"context"
	"net"
	"syscall"
)

type peerCredentials struct {
	UID int
	GID int
}

type peerCredentialsContextKey struct{}

func connPeerCredentials(conn net.Conn) (peerCredentials, bool) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return peerCredentials{}, false
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return peerCredentials{}, false
	}
	var credentials peerCredentials
	var found bool
	_ = rawConn.Control(func(fd uintptr) {
		ucred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			return
		}
		credentials = peerCredentials{UID: int(ucred.Uid), GID: int(ucred.Gid)}
		found = true
	})
	return credentials, found
}

func peerCredentialsFromContext(ctx context.Context) (peerCredentials, bool) {
	credentials, ok := ctx.Value(peerCredentialsContextKey{}).(peerCredentials)
	return credentials, ok
}
