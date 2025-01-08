package http

import (
	"context"
	"net"
	syshttp "net/http"
)

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (self *clientImpl) WithDialer(dialFn DialContextFunc) Client {
	tranport := self.Client.Transport.(*syshttp.Transport)
	tranport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialFn(ctx, network, addr)
	}
	return self
}
