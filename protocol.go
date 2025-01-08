package http

import (
	"context"
	"sync"
)

var protocolResolver = new(sync.Map)

type URLRewriter func(ctx context.Context, urlstr string) string

func RegisterRewriter(proto string, w URLRewriter) {
	protocolResolver.Store(proto, w)
}
