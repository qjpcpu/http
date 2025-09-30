package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type options struct {
	Middlewares []Middleware
}

type Option func(*options)

/* private methods */
func newOptions() *options {
	return &options{}
}

/* option middlewares */
func WithMiddleware(m Middleware) Option {
	return func(opt *options) {
		opt.Middlewares = append(opt.Middlewares, m)
	}
}

func WithPrependMiddleware(m Middleware) Option {
	return func(opt *options) {
		opt.Middlewares = append([]Middleware{m}, opt.Middlewares...)
	}
}

func WithBeforeHook(hook func(*http.Request)) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			hook(req)
			return next(req)
		}
	})
}

func WithTimeout(tm time.Duration) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).Timeout = tm
			return next(req)
		}
	})
}

func WithRetry(opt RetryOption) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).RetryOption = &opt
			return next(req)
		}
	})
}

func WithAfterHook(hook func(*http.Response)) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			res, err := next(req)
			if err == nil && res != nil {
				hook(res)
			}
			return res, err
		}
	})
}

func WithHeaders(hdr map[string]string) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			setRequestHeader(req, hdr)
			return next(req)
		}
	})
}

func WithoutQuery(k string) Option {
	return WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			if qs := req.URL.Query(); qs != nil {
				qs.Del(k)
				req.URL.RawQuery = qs.Encode()
			}
			return next(req)
		}
	})
}

func WithHeader(k, v string) Option {
	return WithHeaders(map[string]string{k: v})
}

type RetryHook func(*http.Request, int)

type RetryOption struct {
	RetryMax      int
	RetryWaitMin  time.Duration                                  // optional
	RetryWaitMax  time.Duration                                  // optional
	CheckResponse func(*http.Response, error) (shouldRetry bool) // optional
}

func setRequestHeader(req *http.Request, header map[string]string) {
	for k, v := range header {
		req.Header.Set(k, v)
		if strings.ToLower(k) == "host" {
			req.Host = v
		}
	}
}

func JSONReader(object any) io.Reader {
	if object == nil {
		return nil
	}
	if reader, ok := object.(io.Reader); ok {
		return reader
	}
	bs, err := json.Marshal(object)
	if err != nil {
		return errReader{err: err}
	}
	return bytes.NewBuffer(bs)
}

type errReader struct{ err error }

func (err errReader) Read([]byte) (int, error) {
	return 0, err.err
}
