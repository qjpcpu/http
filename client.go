package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"
)

const defaultConnectTimeout = 15 * time.Second

// NewClient creates a new HTTP client with a default pooled transport and a 15-second timeout.
func NewClient() Client {
	cli := &clientImpl{
		transport: DefaultPooledTransport(),
	}
	return cli
}

// clientImpl is the concrete implementation of the Client interface.
type clientImpl struct {
	transport *http.Transport
	// middlewares is the chain of client-level middlewares.
	middlewares []Middleware
}

// Fork creates a new client instance. If withMiddlewares is true, it performs a shallow copy
// of the existing client's middlewares to the new instance.
func (client *clientImpl) Fork(withMiddlewares bool) Client {
	cli := &clientImpl{
		transport: client.transport,
	}
	if withMiddlewares {
		ms := make([]Middleware, len(client.middlewares))
		copy(ms, client.middlewares)
		cli.middlewares = ms
	}
	return cli
}

// SetTimeout adds a middleware that sets a default timeout for all requests made by this client.
func (client *clientImpl) SetTimeout(tm time.Duration) Client {
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).Timeout = tm
			return next(req)
		}
	})
	return client
}

// DisableKeepAlive configures the underlying transport to disable HTTP keep-alives.
func (client *clientImpl) DisableKeepAlive(disable bool) Client {
	client.transport.DisableKeepAlives = disable
	return client
}

// SetMock adds a middleware that intercepts requests and returns a mocked response.
func (client *clientImpl) SetMock(fn Endpoint) Client {
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).Mock = fn
			return next(req)
		}
	})
	return client
}

// SetDebug adds a middleware that sets a logger for debugging request and response details.
func (client *clientImpl) SetDebug(w HTTPLogger) Client {
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).Debugger = w
			return next(req)
		}
	})
	return client
}

// SetRetry adds a middleware that sets a default retry policy for all requests.
func (client *clientImpl) SetRetry(opt RetryOption) Client {
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			getValue(req).RetryOption = &opt
			return next(req)
		}
	})
	return client
}

// SetHeader is a convenience method to set a single default header for all requests.
func (client *clientImpl) SetHeader(name, val string) Client {
	return client.SetHeaders(map[string]string{name: val})
}

// SetHeaders adds a middleware that sets multiple default headers for all requests.
func (client *clientImpl) SetHeaders(hder map[string]string) Client {
	return client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			setRequestHeader(req, hder)
			return next(req)
		}
	})
}

// AddMiddleware appends one or more middlewares to the end of the client's middleware chain.
func (client *clientImpl) AddMiddleware(m ...Middleware) Client {
	client.middlewares = append(client.middlewares, m...)
	return client
}

// PrependMiddleware adds one or more middlewares to the beginning of the client's middleware chain.
func (client *clientImpl) PrependMiddleware(m ...Middleware) Client {
	client.middlewares = append(m, client.middlewares...)
	return client
}

// AddBeforeHook adds a middleware that executes a hook function before the request is sent.
func (client *clientImpl) AddBeforeHook(hook func(*http.Request)) Client {
	return client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			hook(req)
			return next(req)
		}
	})
}

// AddAfterHook adds a middleware that executes a hook function after a successful response is received.
func (client *clientImpl) AddAfterHook(hook func(*http.Response)) Client {
	return client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			res, err := next(req)
			if err == nil && res != nil {
				hook(res)
			}
			return res, err
		}
	})
}

// MakeDoer constructs an Endpoint function (which satisfies the Doer interface) by applying
// all client-level and request-level option middlewares.
func (client *clientImpl) MakeDoer(opts ...Option) Doer {
	return (Doer)(client.makeFinalHandler(client.getOptionMiddlewares(opts...)...))
}

// DoRequest executes a pre-constructed http.Request using the client's configuration and
// any additional per-request options.
func (client *clientImpl) DoRequest(req *http.Request, opts ...Option) *Response {
	res, err := client.makeFinalHandler(client.getOptionMiddlewares(opts...)...)(req)
	return buildResponse(req.Context(), res, err)
}

// Do is the core method for creating and executing an HTTP request.
func (client *clientImpl) Do(ctx context.Context, method string, uri string, body io.Reader, opts ...Option) *Response {
	uri = client.rewriteURL(ctx, uri)
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return buildResponse(ctx, nil, err)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	res, err := client.makeFinalHandler(client.getOptionMiddlewares(opts...)...)(req)
	return buildResponse(ctx, res, err)
}

// rewriteURL checks if the URL has a custom protocol scheme and rewrites it if a rewriter is registered.
func (client *clientImpl) rewriteURL(ctx context.Context, urlstr string) string {
	if i := strings.Index(urlstr, "://"); i >= 0 {
		protocol := urlstr[:i]
		if fn, ok := protocolResolver.Load(protocol); ok {
			return fn.(URLRewriter)(ctx, urlstr)
		}
	}
	return urlstr
}

// Download is a convenience method for GET requests that writes the response body directly to an io.Writer.
func (client *clientImpl) Download(ctx context.Context, uri string, w io.Writer, opts ...Option) error {
	return client.Do(ctx, "GET", uri, nil, opts...).Save(w)
}

// Get is a convenience method for making a GET request.
func (client *clientImpl) Get(ctx context.Context, uri string, opts ...Option) *Response {
	return client.Do(ctx, "GET", uri, nil, opts...)
}

// Post is a convenience method for making a POST request with an io.Reader body.
func (client *clientImpl) Post(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response {
	return client.Do(ctx, "POST", urlstr, data, opts...)
}

// Delete is a convenience method for making a DELETE request with an io.Reader body.
func (client *clientImpl) Delete(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response {
	return client.Do(ctx, "DELETE", urlstr, data, opts...)
}

// Put is a convenience method for making a PUT request with an io.Reader body.
func (client *clientImpl) Put(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response {
	return client.Do(ctx, "PUT", urlstr, data, opts...)
}

// PostForm is a convenience method for making a POST request with "application/x-www-form-urlencoded" data.
// It automatically sets the Content-Type header.
func (client *clientImpl) PostForm(ctx context.Context, urlstr string, data map[string]any, opts ...Option) *Response {
	values := url.Values{}
	for k, v := range data {
		values.Set(k, fmt.Sprint(v))
	}
	opts = append([]Option{WithHeader("Content-Type", "application/x-www-form-urlencoded")}, opts...)
	return client.Post(ctx, urlstr, strings.NewReader(values.Encode()), opts...)
}

// PostJSON is a convenience method for making a POST request with a JSON body.
// It handles various data types (string, []byte, io.Reader, or any marshallable struct) and sets the Content-Type header.
func (c *clientImpl) PostJSON(ctx context.Context, urlstr string, data any, opts ...Option) *Response {
	var payload io.Reader
	switch d := data.(type) {
	case string:
		payload = strings.NewReader(d)
	case []byte:
		payload = bytes.NewBuffer(d)
	case json.RawMessage:
		payload = bytes.NewBuffer(d)
	case nil:
		// do nothing
	case io.Reader:
		payload = d
	default:
		bs, err := json.Marshal(data)
		if err != nil {
			return buildResponse(ctx, nil, err)
		}
		payload = bytes.NewBuffer(bs)
	}
	opts = append([]Option{WithHeader("Content-Type", "application/json; charset=utf-8")}, opts...)
	return c.Post(ctx, urlstr, payload, opts...)
}

// makeFinalHandler constructs the final request-processing endpoint by chaining all middlewares.
// The order of execution is:
// 1. `middlewareInitCtx` (always first to ensure context exists)
// 2. Client-level middlewares (in reverse order of addition)
// 3. Request-level (Option) middlewares (in reverse order of addition)
// 4. `middlewareContext` (applies timeout, retry, debug, etc.)
// 5. The actual `client.Client.Do` call.
func (client *clientImpl) makeFinalHandler(extraMiddlewares ...Middleware) Endpoint {
	next := func(req *http.Request) (*http.Response, error) {
		// The final step in the middleware chain is to execute the request.
		// **Design Rationale: Why use a pooled http.Client with a Timeout field?**
		//
		// This design is crucial for ensuring that request timeouts and TCP connection reuse (Keep-Alive)
		// work together correctly and robustly.
		//
		// 1. **Leveraging `setRequestCancel`**: When an `http.Client` has a non-zero `Timeout`, its `Do`
		//    method calls an internal function `setRequestCancel`. This function sets up a timer. If the
		//    request takes too long, the timer triggers `transport.CancelRequest(req)`. This is a specific
		//    cancellation signal that the `http.Transport` is designed to understand perfectly.
		//
		// 2. **Guaranteed Connection Cleanup**: Upon receiving a `CancelRequest` signal, the `Transport`
		//    knows it's a client-initiated cancellation. It will then safely interrupt the request while
		//    ensuring the underlying TCP connection is properly "drained" (reading and discarding any
		//    remaining response body) before returning it to the connection pool. This guarantees
		//    the connection is clean and ready for reuse.
		//
		// 3. **Avoiding Ambiguity**: In contrast, relying solely on a request's `context` for timeouts
		//    can be less reliable in some edge cases. When only the `context` is canceled, the `Transport`
		//    sees a more generic cancellation signal. Under certain network conditions or with non-standard
		//    server behaviors, the `Transport` might conservatively decide to close the connection instead
		//    of attempting to reuse it, to avoid state corruption.
		//
		// 4. **Best Practice**: Therefore, creating a temporary `http.Client` for each request and setting
		//    its `Timeout` field is the most robust way to handle per-request timeouts in Go. It ensures
		//    maximum connection reuse and prevents resource leaks, which is why this library adopts this
		//    pattern using a `sync.Pool` for efficiency.
		timeout := defaultConnectTimeout // Fallback to default connect timeout
		gv := getValue(req)
		if gv != nil && gv.Timeout != timeoutNotSet {
			timeout = gv.Timeout
		}
		c := poolGetClient(client.transport, timeout)
		defer poolPutClient(c)
		return c.Do(req)
	}

	next = middlewareContext(next)

	// Apply all middlewares in reverse order to create the chain.
	for i := len(extraMiddlewares) - 1; i >= 0; i-- {
		next = extraMiddlewares[i](next)
	}
	for i := len(client.middlewares) - 1; i >= 0; i-- {
		next = client.middlewares[i](next)
	}
	// This middleware must be the outermost one to initialize the request context value.
	next = middlewareInitCtx(next)

	return next
}

// getOptionMiddlewares processes a slice of Option functions and returns the resulting slice of middlewares.
func (client *clientImpl) getOptionMiddlewares(opts ...Option) []Middleware {
	opt := newOptions()
	for _, fn := range opts {
		fn(opt)
	}
	return opt.Middlewares
}

// SetMaxIdleConns configures the maximum number of idle connections for the underlying transport.
func (client *clientImpl) SetMaxIdleConns(maxIdleConn int) Client {
	if maxIdleConn > 0 {
		client.transport.MaxIdleConns = maxIdleConn
	}
	return client
}

// SetIdleConnTimeout configures the idle connection timeout for the underlying transport.
func (client *clientImpl) SetIdleConnTimeout(idleTimeout time.Duration) Client {
	if idleTimeout > 0 {
		client.transport.IdleConnTimeout = idleTimeout
	}
	return client
}

// Doer is an adapter type that allows an Endpoint function to be used as an http.RoundTripper.
type Doer func(*http.Request) (*http.Response, error)

// Do satisfies the http.RoundTripper interface.
func (hd Doer) Do(req *http.Request) (*http.Response, error) {
	return hd(req)
}

// DefaultPooledTransport creates a new http.Transport with sensible defaults for a pooled,
// long-lived client. It includes settings for keep-alives, timeouts, and connection pooling.
func DefaultPooledTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultConnectTimeout,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
	}
	return transport
}

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (client *clientImpl) WithDialer(dialFn DialContextFunc) Client {
	client.transport.DialContext = dialFn
	return client
}

var clientPool = sync.Pool{
	New: func() any {
		return &http.Client{}
	},
}

func poolGetClient(tr *http.Transport, tm time.Duration) *http.Client {
	c := clientPool.Get().(*http.Client)
	c.Transport = tr
	c.CheckRedirect = nil
	c.Jar = nil
	c.Timeout = tm
	return c
}

func poolPutClient(c *http.Client) {
	c.Transport = nil
	c.CheckRedirect = nil
	c.Jar = nil
	c.Timeout = 0
	clientPool.Put(c)
}
