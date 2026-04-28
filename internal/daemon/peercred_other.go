//go:build !linux

package daemon

import (
	"context"
	"net"
)

type peerCredentials struct {
	UID int
	GID int
}

type peerCredentialsContextKey struct{}

func connPeerCredentials(conn net.Conn) (peerCredentials, bool) {
	return peerCredentials{}, false
}

func peerCredentialsFromContext(ctx context.Context) (peerCredentials, bool) {
	return peerCredentials{}, false
}
