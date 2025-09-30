package http

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Client defines the interface for an HTTP client.
type Client interface {
	// SetTimeout sets the default request timeout for the client.
	SetTimeout(tm time.Duration) Client
	// DisableKeepAlive sets whether to disable HTTP keep-alives.
	DisableKeepAlive(disable bool) Client
	// SetMock sets a mock function to intercept all requests and return a predefined response, primarily for testing.
	SetMock(fn Endpoint) Client
	// SetDebug sets a debugger (Logger) to print detailed request and response logs.
	SetDebug(w HTTPLogger) Client
	// SetRetry sets the default retry policy for the client.
	SetRetry(opt RetryOption) Client
	// SetHeader sets a default header that will be sent with all requests.
	SetHeader(name, val string) Client
	// SetHeaders sets multiple default headers that will be sent with all requests.
	SetHeaders(hder map[string]string) Client
	// AddMiddleware appends one or more middlewares to the client. They execute in the order they are added.
	AddMiddleware(m ...Middleware) Client
	// PrependMiddleware prepends one or more middlewares to the client. They execute before existing middlewares.
	PrependMiddleware(m ...Middleware) Client
	// AddBeforeHook adds a hook function that executes before a request is sent.
	AddBeforeHook(hook func(*http.Request)) Client
	// AddAfterHook adds a hook function that executes after a successful response is received.
	AddAfterHook(hook func(*http.Response)) Client
	// MakeDoer creates a Doer based on the provided options. A Doer is a function that can execute an http.Request, useful for integration with other libraries.
	MakeDoer(opts ...Option) Doer
	// DoRequest executes a pre-created http.Request using the client's configuration and specified options.
	DoRequest(req *http.Request, opts ...Option) *Response
	// Do is the core method for executing an HTTP request. It allows specifying the
	// HTTP method, URL, request body, and per-request options.
	//
	// Middleware Execution Order:
	// The client uses a powerful middleware pattern. Understanding the execution order is key:
	//
	// 1. Client-level Middlewares: Middlewares added via `AddMiddleware` or `SetHeader`, `SetTimeout`
	//    on the client are executed first, in the order they were added. Middlewares added
	//    via `PrependMiddleware` run before those from `AddMiddleware`.
	//
	// 2. Request-level (Option) Middlewares: Middlewares provided via `opts...` (e.g., `WithTimeout`,
	//    `WithHeader`) are executed AFTER all client-level middlewares. This allows per-request
	//    options to override client-wide defaults.
	//
	// The final step is the actual HTTP request execution, which is also wrapped by internal middlewares that apply timeout, retry, and logging logic based on the configuration accumulated from the previous middleware layers.
	Do(ctx context.Context, method string, uri string, body io.Reader, opts ...Option) *Response
	// Download is a convenience method for downloading a resource and writing its content to an io.Writer.
	Download(ctx context.Context, uri string, w io.Writer, opts ...Option) error
	// Get is a convenience method for executing a GET request.
	Get(ctx context.Context, uri string, opts ...Option) *Response
	// Post is a convenience method for executing a POST request with an io.Reader body.
	Post(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response
	// Delete is a convenience method for executing a DELETE request with an io.Reader body.
	Delete(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response
	// Put is a convenience method for executing a PUT request with an io.Reader body.
	Put(ctx context.Context, urlstr string, data io.Reader, opts ...Option) *Response
	// PostForm is a convenience method for sending a POST request with "application/x-www-form-urlencoded" format.
	PostForm(ctx context.Context, urlstr string, data map[string]any, opts ...Option) *Response
	// PostJSON is a convenience method for sending a POST request with a JSON body.
	// It automatically sets the "Content-Type" header to "application/json; charset=utf-8".
	// The `data` parameter can be of various types:
	//   - A struct or map: It will be marshaled into a JSON object using `json.Marshal`.
	//   - A string, []byte, or json.RawMessage: It will be sent as the raw request body.
	//   - An io.Reader: The stream's content will be sent as the request body.
	//   - nil: An empty request body will be sent.
	PostJSON(ctx context.Context, urlstr string, data any, opts ...Option) *Response
	// WithDialer allows setting a custom dialer function for the client's Transport.
	WithDialer(dialFn DialContextFunc) Client
	// Fork creates a new "child" client instance that shares the parent's underlying
	// http.Transport. This is highly efficient as it allows connection pooling and reuse
	// across multiple, logically distinct clients.
	//
	// Use Case:
	// Create a "template" client with common configurations (e.g., User-Agent, default timeout,
	// retry policy). Then, fork it to create specialized clients for different services,
	// each with its own specific settings (e.g., auth tokens, shorter timeouts) without
	// losing the performance benefits of a shared connection pool.
	//
	// If withMiddlewares is true, the new client inherits a copy of the parent's middlewares.
	// If false, the new client starts with a clean middleware chain.
	Fork(withMiddlewares bool) Client
	// SetMaxIdleConns sets the maximum number of idle connections for the Transport.
	SetMaxIdleConns(maxIdleConn int) Client
	// SetIdleConnTimeout sets the idle connection timeout for the Transport.
	SetIdleConnTimeout(idleTimeout time.Duration) Client
}
