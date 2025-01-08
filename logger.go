package http

import (
	"context"
	"io"
	lg "log"
)

var logger = lg.New(io.Discard, "http", lg.LstdFlags)

func log(_ context.Context, format string, args ...interface{}) {
	logger.Printf(format+"\n", args...)
}
