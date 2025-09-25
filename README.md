# Go HTTP Client

[![Go Report Card](https://goreportcard.com/badge/github.com/qjpcpu/http)](https://goreportcard.com/report/github.com/qjpcpu/http)
[![GoDoc](https://godoc.org/github.com/qjpcpu/http?status.svg)](https://godoc.org/github.com/qjpcpu/http)

A powerful, flexible, and middleware-driven HTTP client for Go.

This library enhances Go's standard `net/http` client with a fluent interface, robust middleware architecture, and out-of-the-box features like automatic retries, detailed logging, and easy mocking. It's designed to make HTTP requests simple, readable, and resilient.

## Features

- **Middleware Architecture**: Easily add functionality like logging, retries, and auth to clients or individual requests.
- **Fluent Interface**: Configure your client with a clean, chainable API.
- **Automatic Retries**: Built-in support for exponential backoff and jitter to handle transient network issues.
- **Request & Response Debugging**: Detailed logging of requests and responses for easy debugging.
- **Effortless Mocking**: Mock server responses for reliable and fast unit tests.
- **Connection Pooling & Keep-Alive**: High performance by default, with efficient connection reuse.
- **Helper Methods**: Convenient methods for common tasks like `PostJSON`, `PostForm`, and `Download`.
- **Context-Aware**: Full support for `context.Context` for cancellation and deadlines.

## Installation

```bash
go get github.com/qjpcpu/http
```

## Usage

### Basic GET Request

```go
package main

import (
	"context"
	"fmt"
	"github.com/qjpcpu/http"
)

func main() {
	client := http.NewClient()
	
	// Make a GET request
	res, err := client.Get(context.Background(), "https://httpbin.org/get").GetBody()
	if err != nil {
		panic(err)
	}
	
	fmt.Println(string(res))
}
```

### POST JSON Data

```go
client := http.NewClient()

payload := map[string]string{
	"name": "Gemini",
	"type": "Code Assistant",
}

var result map[string]interface{}

err := client.PostJSON(context.Background(), "https://httpbin.org/post", payload).Unmarshal(&result)
if err != nil {
	panic(err)
}

fmt.Printf("JSON Response: %+v\n", result["json"])
```

### Using Middlewares

Middlewares are the core of this library. You can add them to the client for global effect or to a single request.

#### Adding a Global Header

```go
client := http.NewClient().
	SetHeader("User-Agent", "my-awesome-app/1.0").
	SetHeader("X-Request-ID", "global-id-123")

// All requests from this client will now include these headers.
client.Get(context.Background(), "https://httpbin.org/headers")
```

#### Per-Request Headers and Timeout

Options passed to `Get`, `Post`, etc., will override client-level settings for that specific request.

```go
client := http.NewClient().SetTimeout(10 * time.Second)

// This request will have a 2-second timeout and a custom header,
// overriding the client's default 10-second timeout.
err := client.Get(
	context.Background(), 
	"https://httpbin.org/delay/3",
	http.WithTimeout(2*time.Second),
	http.WithHeader("X-Custom-Header", "per-request-value"),
).Err

if err != nil {
	fmt.Println("Request failed as expected:", err)
}
```

### Automatic Retries

Configure the client to automatically retry failed requests.

```go
client := http.NewClient().SetRetry(http.RetryOption{
	RetryMax:      3, // Max 3 retries
	RetryWaitMin:  100 * time.Millisecond,
	RetryWaitMax:  2 * time.Second,
	CheckResponse: func(res *http.Response, err error) bool {
		// Retry on network errors or 5xx server errors.
		return err != nil || (res != nil && res.StatusCode >= 500)
	},
})

// This request will be tried up to 4 times (1 initial + 3 retries).
client.Get(context.Background(), "https://httpbin.org/status/503")
```

### Debugging

Enable detailed logging to inspect requests and responses.

```go
client := http.NewClient().SetDebug(http.DefaultLogger)

client.PostJSON(context.Background(), "https://httpbin.org/post", map[string]string{"hello": "world"})

/*
Output:
[POST] https://httpbin.org/post 200 OK reqat:2023-10-27 10:30:00.123 cost:250ms
[Request-Headers]
  Content-Type:application/json; charset=utf-8
  User-Agent:Go-http-client/1.1
...
[Request-Body]
{"hello":"world"}
[Response-Headers]
  Content-Type:application/json
...
[Response-Body]
{...}
*/
```

### Mocking for Tests

Easily mock responses in your unit tests without making real network calls.

```go
func TestMyFunction(t *testing.T) {
	client := http.NewClient()
	
	// Set a mock response for any request.
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		// You can add logic here to return different responses based on the request.
		mockResponse := `{"message": "success"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(mockResponse)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})

	// Your function that uses the client
	body, err := client.Get(context.Background(), "http://any-url.com/api/data").GetBody()
	
	// Assertions
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if string(body) != `{"message": "success"}` {
		t.Errorf("Expected mock body, got %s", string(body))
	}
}
```

## API Overview

### Client Creation
- `http.NewClient() Client`

### Client Configuration (chainable)
- `SetTimeout(time.Duration) Client`
- `SetHeader(string, string) Client`
- `SetRetry(RetryOption) Client`
- `SetDebug(HTTPLogger) Client`
- `SetMock(Endpoint) Client`
- `AddMiddleware(Middleware) Client`
- `Fork(bool) Client`

### Request Execution
- `Get(ctx, url, ...Option) *Response`
- `Post(ctx, url, data, ...Option) *Response`
- `PostJSON(ctx, url, data, ...Option) *Response`
- `PostForm(ctx, url, data, ...Option) *Response`
- `Put(...)`, `Delete(...)`
- `Download(ctx, url, writer, ...Option) error`
- `Do(ctx, method, url, body, ...Option) *Response`

### Response Handling
- `response.Err error`
- `response.GetBody() ([]byte, error)`
- `response.Unmarshal(interface{}) error`
- `response.Save(io.Writer) error`
