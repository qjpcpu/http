package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func elementsMatch(t *testing.T, listA, listB interface{}) {
	t.Helper()
	valA := reflect.ValueOf(listA)
	valB := reflect.ValueOf(listB)

	if valA.Len() != valB.Len() {
		t.Fatalf("slice lengths are not equal: %d != %d", valA.Len(), valB.Len())
	}

	// This is a simplified version for []int. A more generic one would be more complex.
	sortedA := make([]int, valA.Len())
	sortedB := make([]int, valB.Len())
	for i := 0; i < valA.Len(); i++ {
		sortedA[i] = valA.Index(i).Interface().(int)
		sortedB[i] = valB.Index(i).Interface().(int)
	}
	sort.Ints(sortedA)
	sort.Ints(sortedB)

	if !reflect.DeepEqual(sortedA, sortedB) {
		t.Fatalf("slices do not contain the same elements. got: %v, want: %v", listB, listA)
	}
}

func TestSetMock(t *testing.T) {
	client := NewClient()
	res := &http.Response{}
	var val int
	client.SetMock(func(*http.Request) (*http.Response, error) {
		val = 1
		return res, nil
	})

	res1 := client.Get(nil, "http://ssssss")
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if val != 1 {
		t.Fatalf("expected val to be 1, got %d", val)
	}
	if res1.Response != res {
		t.Fatalf("expected response to be %v, got %v", res, res1.Response)
	}
}

func TestMiddleware(t *testing.T) {
	client := NewClient()
	var val int
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		val = 1
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()

	var slice []int
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice = append(slice, 1)
			a, b := next(req)
			slice = append(slice, 2)
			return a, b
		}
	})
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice = append(slice, 3)
			return next(req)
		}
	})

	res1 := client.Get(nil, server.URLPrefix+"/hello")
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if val != 1 {
		t.Fatalf("expected val to be 1, got %d", val)
	}
	elementsMatch(t, []int{1, 3, 2}, slice)
}

func TestResponse(t *testing.T) {
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"a":1,"b":"HELLO"}`))
	})
	defer server.ServeBackground()()

	client := NewClient()

	res := struct {
		A int    `json:"a"`
		B string `json:"b"`
	}{}
	res1 := client.Post(nil, server.URLPrefix+"/hello", nil)
	if err := res1.Unmarshal(&res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if res.A != 1 {
		t.Fatalf("expected res.A to be 1, got %d", res.A)
	}
	if res.B != "HELLO" {
		t.Fatalf("expected res.B to be 'HELLO', got %q", res.B)
	}
}

func TestGet(t *testing.T) {
	server := NewMockServer().Handle("/get", func(w http.ResponseWriter, req *http.Request) {
		args := make(map[string]string)
		qs := req.URL.Query()
		for k := range qs {
			args[k] = qs.Get(k)
		}
		data, _ := json.Marshal(map[string]interface{}{
			"args": args,
		})
		w.Write(data)
	})
	defer server.ServeBackground()()

	client := NewClient()
	res := struct {
		Args struct {
			A string `json:"a"`
		} `json:"args"`
	}{}
	res1 := client.Get(nil, server.URLPrefix+"/get?a=hello")
	if err := res1.Unmarshal(&res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if res.Args.A != "hello" {
		t.Fatalf("expected res.Args.A to be 'hello', got %q", res.Args.A)
	}
}

func interceptStdout() func() []byte {
	stdout := os.Stdout
	stderr := os.Stderr
	fname := filepath.Join(os.TempDir(), "stdout")
	temp, _ := os.Create(fname)
	os.Stdout = temp
	os.Stderr = temp
	return func() []byte {
		temp.Sync()
		data, _ := os.ReadFile(fname)
		temp.Close()
		os.Remove(fname)
		os.Stderr = stderr
		os.Stdout = stdout
		return data
	}
}

func TestDebug(t *testing.T) {
	stdout := interceptStdout()
	server := NewMockServer()
	defer server.ServeBackground()()

	client := NewClient().SetDebug(DefaultLogger)
	res := struct {
		A int    `json:"a"`
		B string `json:"b"`
	}{
		A: 100,
		B: "HELLO",
	}
	res1 := client.PostJSON(nil, server.URLPrefix+"/echo", res)
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if err := res1.Unmarshal(&res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	out := string(stdout())

	for _, substr := range []string{
		server.URLPrefix + "/echo",
		`200 OK`,
		`"{\"a\":100,\"b\":\"HELLO\"}"`,
		`application/json`,
		`Response-Headers`,
		`Request-Body`,
		`[Response-Body]`,
	} {
		if !strings.Contains(out, substr) {
			t.Errorf("expected stdout to contain %q", substr)
		}
	}
}

func TestDebugWithErr(t *testing.T) {
	stdout := interceptStdout()
	client := NewClient().SetDebug(DefaultLogger)
	client.SetMock(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("user error")
	})
	res := struct {
		A int    `json:"a"`
		B string `json:"b"`
	}{
		A: 100,
		B: "HELLO",
	}
	res1 := client.PostJSON(nil, "http://wwws", res)
	if res1.Error() == nil {
		t.Fatal("expected an error, but got nil")
	}
	out := string(stdout())
	if !strings.Contains(out, `[Response Error]`) {
		t.Errorf("expected stdout to contain %q", `[Response Error]`)
	}
	if !strings.Contains(out, `user error`) {
		t.Errorf("expected stdout to contain %q", `user error`)
	}
}

func TestRepeatableRead(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(*http.Request) (*http.Response, error) {
		return res, nil
	})
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			res, err := next(req)
			RepeatableReadResponse(res)
			RepeatableReadResponse(res)
			RepeatableReadResponse(res)
			return res, err
		}
	})

	res1, err := client.Get(nil, "http://sss").GetBody()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if body := string(res1); body != "HELLO" {
		t.Fatalf("expected body to be 'HELLO', got %q", body)
	}
}

func TestRepeatableReadRequest(t *testing.T) {
	client := NewClient()
	reqBody := `GOGOGOGOGOGO`
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		data, e := io.ReadAll(req.Body)
		if e != nil {
			t.Fatalf("ReadAll in mock failed: %v", e)
		}
		if string(data) != reqBody {
			t.Fatalf("expected request body %q, got %q", reqBody, string(data))
		}
		return res, nil
	})
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			data, e := RepeatableReadRequest(req)
			if e != nil {
				t.Fatalf("RepeatableReadRequest failed: %v", e)
			}
			if string(data) != reqBody {
				t.Fatalf("expected repeatable body %q, got %q", reqBody, string(data))
			}
			RepeatableReadRequest(req)
			return next(req)
		}
	})

	res1 := client.Post(nil, "http://sss", []byte(reqBody))
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
}

func TestGlobalHeader(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	hdl := make(map[string]string)
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		for k := range req.Header {
			hdl[strings.ToLower(k)] = req.Header.Get(k)
		}
		return res, nil
	})
	client.SetHeader("AA", "BB")

	res1 := client.Get(nil, "http://sss")
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if hdl["aa"] != "BB" {
		t.Fatalf("expected header 'aa' to be 'BB', got %q", hdl["aa"])
	}
}

func TestOptionMiddleware(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return res, nil
	})
	var val int
	mid := func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			val++
			return next(req)
		}
	}
	res1 := client.Get(nil, "http://sss", WithMiddleware(mid))
	if res1.Error() != nil {
		t.Fatalf("expected nil error on first call, got %v", res1.Error())
	}
	res1 = client.Get(nil, "http://sss")
	if res1.Error() != nil {
		t.Fatalf("expected nil error on second call, got %v", res1.Error())
	}
	/* execute once */
	if val != 1 {
		t.Fatalf("expected middleware to be called once, got %d", val)
	}
}

type httpClientor interface {
	Do(*http.Request) (*http.Response, error)
}

func runHTTP(h httpClientor) (*http.Response, error) {
	req, _ := http.NewRequest("GET", "http://sss", nil)
	return h.Do(req)
}

func TestDoer(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return res, nil
	})
	var val int
	mid := func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			val++
			return next(req)
		}
	}
	doer := client.MakeDoer(WithMiddleware(mid))
	_, err := runHTTP(doer)
	if err != nil {
		t.Fatalf("runHTTP failed: %v", err)
	}
	if val != 1 {
		t.Fatalf("expected middleware to be called once, got %d", val)
	}
}

func TestBeforeHook(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return res, nil
	})
	var val int
	res1 := client.Get(nil, "http://sss", WithBeforeHook(func(*http.Request) { val++ }))
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if val != 1 {
		t.Fatalf("expected hook to be called once, got %d", val)
	}
}

func TestAfterHook(t *testing.T) {
	client := NewClient()
	res := &http.Response{
		Body: io.NopCloser(strings.NewReader("HELLO")),
	}
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return res, nil
	})
	var val int
	res1 := client.Get(nil, "http://sss", WithAfterHook(func(*http.Response) { val++ }))
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if val != 1 {
		t.Fatalf("expected hook to be called once, got %d", val)
	}
}

func TestTimeout(t *testing.T) {
	stopChan := make(chan struct{}, 1)
	server := NewMockServer().Handle("/delay", func(w http.ResponseWriter, req *http.Request) {
		select {
		case <-time.After(1 * time.Hour):
		case <-stopChan:
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	client.SetTimeout(1 * time.Millisecond)
	res := client.Get(nil, server.URLPrefix+"/delay")
	if res.Error() == nil {
		t.Fatal("expected an error, but got nil")
	}
	if !strings.Contains(res.Error().Error(), "context deadline exceeded") {
		t.Fatalf("expected error to contain 'context deadline exceeded', got %q", res.Error().Error())
	}

	close(stopChan)
}

func TestTimeoutOverwrite(t *testing.T) {
	stopChan := make(chan struct{}, 1)
	server := NewMockServer().Handle("/delay", func(w http.ResponseWriter, req *http.Request) {
		select {
		case <-time.After(30 * time.Millisecond):
		case <-stopChan:
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	client.SetTimeout(100 * time.Hour)
	err := client.Get(nil, server.URLPrefix+"/delay", WithTimeout(time.Millisecond)).Error()
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected error to contain 'context deadline exceeded' or 'timeout', got %q", err.Error())
	}
	close(stopChan)
}

func TestTimeoutOverwrite2(t *testing.T) {
	stopChan := make(chan struct{}, 1)
	server := NewMockServer().Handle("/delay", func(w http.ResponseWriter, req *http.Request) {
		select {
		case <-time.After(3 * time.Millisecond):
		case <-stopChan:
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	client.SetTimeout(1 * time.Millisecond)
	/* should not timeout */
	err := client.Get(nil, server.URLPrefix+"/delay", WithTimeout(time.Hour)).Error()
	if err != nil {
		t.Fatalf("expected no error, but got %v", err)
	}
	close(stopChan)
}

func TestDownload(t *testing.T) {
	body := `"BJLKJLJLJL:JL:JKLJ`
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(body))
	})
	defer server.ServeBackground()()

	client := NewClient()

	buf := &bytes.Buffer{}
	err := client.Download(nil, server.URLPrefix+"/hello", buf)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if buf.String() != body {
		t.Fatalf("expected downloaded content %q, got %q", body, buf.String())
	}
}

func TestMockServer(t *testing.T) {
	body := `"BJLKJLJLJL:JL:JKLJ`
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(body))
	})
	defer server.ServeBackground()()
	client := NewClient()

	res, err := client.Get(nil, server.URLPrefix+"/hello").GetBody()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if bodyStr := string(res); bodyStr != body {
		t.Fatalf("expected body %q, got %q", body, bodyStr)
	}
}

func TestRetryCheckResponse(t *testing.T) {
	var val int
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		val++
		if val < 3 {
			w.Write([]byte("FAIL"))
			return
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()
	client := NewClient()

	res, err := client.Get(nil, server.URLPrefix+"/hello", WithRetry(RetryOption{
		RetryMax:     3,
		RetryWaitMin: 1 * time.Millisecond,
		RetryWaitMax: 3 * time.Millisecond,
		CheckResponse: func(res *http.Response, err error) bool {
			data, _ := RepeatableReadResponse(res)
			return string(data) == "FAIL"
		},
	})).GetBody()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if val != 3 {
		t.Fatalf("expected 3 attempts, got %d", val)
	}
	if body := string(res); body != "OK" {
		t.Fatalf("expected final body to be 'OK', got %q", body)
	}
}

func TestRetryModifyRequest(t *testing.T) {
	var val int
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		val++
		if val > 1 { // On retry
			if !strings.Contains(req.URL.String(), "second") {
				t.Errorf("expected URL to contain 'second' on retry, got: %s", req.URL.String())
			}
			if strings.Contains(req.URL.String(), "first") {
				t.Errorf("URL should not contain 'first' on retry, got: %s", req.URL.String())
			}
		} else { // First attempt
			if !strings.Contains(req.URL.String(), "first") {
				t.Errorf("expected URL to contain 'first' on first attempt, got: %s", req.URL.String())
			}
			if strings.Contains(req.URL.String(), "second") {
				t.Errorf("URL should not contain 'second' on first attempt, got: %s", req.URL.String())
			}
		}
		if val < 3 {
			w.Write([]byte("FAIL"))
			return
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()
	client := NewClient()

	res, err := client.Get(nil, server.URLPrefix+"/hello?args=first", WithRetry(RetryOption{
		RetryMax:     3,
		RetryWaitMin: 1 * time.Millisecond,
		RetryWaitMax: 3 * time.Millisecond,
		CheckResponse: func(res *http.Response, err error) bool {
			data, _ := RepeatableReadResponse(res)
			return string(data) == "FAIL"
		},
	}), WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			FromRequest(req).AddRetryHook(func(nreq *http.Request, i int) {
				qs := nreq.URL.Query()
				qs.Set("args", "second")
				nreq.URL.RawQuery = qs.Encode()
			})
			return next(req)
		}
	})).GetBody()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if val != 3 {
		t.Fatalf("expected 3 attempts, got %d", val)
	}
	if body := string(res); body != "OK" {
		t.Fatalf("expected final body to be 'OK', got %q", body)
	}
}

func TestRetryModifyRequestByPrevMiddleware(t *testing.T) {
	var val int
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		val++
		if val > 1 {
			if !strings.Contains(req.URL.String(), "extra") {
				t.Errorf("expected URL to contain 'extra' on retry, got: %s", req.URL.String())
			}
		} else {
			if !strings.Contains(req.URL.String(), "first") {
				t.Errorf("expected URL to contain 'first' on first attempt, got: %s", req.URL.String())
			}
		}
		if val < 3 {
			w.Write([]byte("FAIL"))
			return
		}
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()
	client := NewClient()
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()
			req = req.WithContext(context.WithValue(ctx, "prev", "extra"))
			return next(req)
		}
	})

	res, err := client.Get(nil, server.URLPrefix+"/hello?args=first", WithRetry(RetryOption{
		RetryMax:     3,
		RetryWaitMin: 1 * time.Millisecond,
		RetryWaitMax: 3 * time.Millisecond,
		CheckResponse: func(res *http.Response, err error) bool {
			data, _ := RepeatableReadResponse(res)
			return string(data) == "FAIL"
		},
	}), WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			FromRequest(req).AddRetryHook(func(nreq *http.Request, i int) {
				if v := nreq.Context().Value("prev"); v != nil {
					qs := nreq.URL.Query()
					qs.Set("args", v.(string))
					nreq.URL.RawQuery = qs.Encode()
				}
			})
			return next(req)
		}
	})).GetBody()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if val != 3 {
		t.Fatalf("expected 3 attempts, got %d", val)
	}
	if body := string(res); body != "OK" {
		t.Fatalf("expected final body to be 'OK', got %q", body)
	}
}

func TestOverwriteRetry(t *testing.T) {
	var val int
	client := NewClient()
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		val++
		return nil, errors.New("err")
	})

	client.AddMiddleware(RetryMiddleware(RetryOption{RetryMax: 2}))

	res := client.Get(nil, "http://hello", WithRetry(RetryOption{
		RetryMax: 0,
	}))
	if res.Error() == nil {
		t.Fatal("expected an error, but got nil")
	}
	if val != 1 {
		t.Fatalf("expected only 1 attempt, got %d", val)
	}
}

func TestSetHeader(t *testing.T) {
	server := NewMockServer().Handle("/header", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Host") == "" {
			req.Header.Set("Host", req.Host)
		}
		data, _ := json.Marshal(req.Header)
		w.Write(data)
	})
	defer server.ServeBackground()()
	client := NewClient()

	client.SetHeader("AA", "BB")

	headers := make(http.Header)
	err := client.Get(nil, server.URLPrefix+"/header", WithHeaders(map[string]string{
		"c":    "eS",
		"host": "sssssss",
	})).Unmarshal(&headers)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if headers.Get("AA") != "BB" {
		t.Errorf("expected header AA to be 'BB', got %q", headers.Get("AA"))
	}
	if headers.Get("c") != "eS" {
		t.Errorf("expected header c to be 'eS', got %q", headers.Get("c"))
	}
	if headers.Get("host") != "sssssss" {
		t.Errorf("expected header host to be 'sssssss', got %q", headers.Get("host"))
	}
}

func TestContextCancel(t *testing.T) {
	body := []byte(strings.Repeat("x", 65535))
	server := NewMockServer().Handle("/header", func(w http.ResponseWriter, req *http.Request) {
		// Simulate a delay to ensure the client has time to process the cancellation.
		time.Sleep(50 * time.Millisecond)
		w.Write(body)
	})
	defer server.ServeBackground()()
	client := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context before the request

	res1 := client.Get(ctx, server.URLPrefix+"/header")
	if res1.Error() == nil {
		t.Fatal("expected an error due to canceled context, but got nil")
	}
	if !strings.Contains(res1.Error().Error(), "context canceled") {
		t.Errorf("expected error to be 'context canceled', got %q", res1.Error().Error())
	}
}

func TestDownload2(t *testing.T) {
	body := []byte(strings.Repeat("x", 65535))
	server := NewMockServer().Handle("/header", func(w http.ResponseWriter, req *http.Request) {
		w.Write(body)
	})
	defer server.ServeBackground()()
	client := NewClient()
	buf := new(bytes.Buffer)
	err := client.Download(nil, server.URLPrefix+"/header", buf)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if buf.Len() != len(body) {
		t.Fatalf("expected downloaded size to be %d, got %d", len(body), buf.Len())
	}
}

func TestStatusCode(t *testing.T) {
	body := []byte(`BODY`)
	server := NewMockServer().Handle("/code", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(500)
		w.Write(body)
	})
	defer server.ServeBackground()()
	client := NewClient().AddMiddleware(MiddlewareSetAllowedStatusCode(http.StatusOK))
	res1 := client.Get(nil, server.URLPrefix+"/code")
	if res1.Error() == nil {
		t.Fatal("expected an error for disallowed status code, but got nil")
	}
	if !strings.Contains(res1.Error().Error(), `500 Internal Server Error BODY`) {
		t.Errorf("error message mismatch, got: %q", res1.Error().Error())
	}

	client = NewClient().AddMiddleware(MiddlewareSetBlockedStatusCode(http.StatusInternalServerError))
	res1 = client.Get(nil, server.URLPrefix+"/code")
	if res1.Error() == nil {
		t.Fatal("expected an error for blocked status code, but got nil")
	}
	if !strings.Contains(res1.Error().Error(), `500 Internal Server Error BODY`) {
		t.Errorf("error message mismatch, got: %q", res1.Error().Error())
	}
}

func TestDropQuery(t *testing.T) {
	server := NewMockServer().Handle("/get", func(w http.ResponseWriter, req *http.Request) {
		args := make(map[string]string)
		qs := req.URL.Query()
		for k := range qs {
			args[k] = qs.Get(k)
		}
		data, _ := json.Marshal(map[string]interface{}{
			"args": args,
		})
		w.Write(data)
	})
	defer server.ServeBackground()()

	client := NewClient()
	res := struct {
		Args struct {
			A string `json:"a"`
			B string `json:"b"`
		} `json:"args"`
	}{}
	res1 := client.Get(nil, server.URLPrefix+"/get?a=hello&b=world", WithoutQuery("b"))
	if err := res1.Unmarshal(&res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if res.Args.B != "" {
		t.Fatalf("expected query param 'b' to be dropped, but got value %q", res.Args.B)
	}
}

type SyncList struct {
	slice []string
	rw    sync.Mutex
}

func NewList() *SyncList {
	return &SyncList{}
}

func (s *SyncList) Append(v string) *SyncList {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.slice = append(s.slice, v)
	return s
}

func (s *SyncList) String() string {
	s.rw.Lock()
	defer s.rw.Unlock()
	return strings.Join(s.slice, ",")
}

func TestMiddlewareSequence(t *testing.T) {
	client := NewClient()
	var val int
	server := NewMockServer().Handle("/hello", func(w http.ResponseWriter, req *http.Request) {
		val = 1
		w.Write([]byte("OK"))
	})
	defer server.ServeBackground()()

	slice := NewList()
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("1")
			return next(req)
		}
	})
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("2")
			return next(req)
		}
	})
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("3")
			return next(req)
		}
	})
	client.PrependMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("4")
			return next(req)
		}
	})

	opt0 := WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("a")
			return next(req)
		}
	})
	opt1 := WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("b")
			return next(req)
		}
	})
	opt2 := WithMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("c")
			return next(req)
		}
	})
	opt3 := WithPrependMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			slice.Append("P")
			return next(req)
		}
	})

	res1 := client.Get(nil, server.URLPrefix+"/hello", opt0, opt1, opt2, opt3)
	if res1.Error() != nil {
		t.Fatalf("expected nil error, got %v", res1.Error())
	}
	if val != 1 {
		t.Fatalf("expected val to be 1, got %d", val)
	}
	if want := "4,1,2,3,P,a,b,c"; slice.String() != want {
		t.Fatalf("middleware sequence mismatch, got %q, want %q", slice.String(), want)
	}
}

func TestClientMethods(t *testing.T) {
	// Test Fork
	client := NewClient()
	var forkVal int
	client.AddMiddleware(func(next Endpoint) Endpoint {
		return func(req *http.Request) (*http.Response, error) {
			forkVal++
			return next(req)
		}
	})
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return &http.Response{Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	forkedWithMiddleware := client.Fork(true)
	forkedWithoutMiddleware := client.Fork(false)

	forkedWithMiddleware.Get(context.Background(), "/test")
	if forkVal != 1 {
		t.Errorf("Expected middleware to run on forked client (with middlewares), forkVal = %d", forkVal)
	}

	forkedWithoutMiddleware.Get(context.Background(), "/test")
	if forkVal != 1 {
		t.Errorf("Expected middleware to NOT run on forked client (without middlewares), forkVal = %d", forkVal)
	}

	// Test DisableKeepAlive
	keepAliveClient := NewClient().DisableKeepAlive(true)
	if c, ok := keepAliveClient.(*clientImpl); !ok || c.transport.DisableKeepAlives != true {
		t.Error("DisableKeepAlive(true) was not set correctly")
	}

	// Test SetMaxIdleConns & SetIdleConnTimeout
	transportClient := NewClient().SetMaxIdleConns(50).SetIdleConnTimeout(15 * time.Second)
	if c, ok := transportClient.(*clientImpl); !ok {
		t.Fatal("Could not cast client to clientImpl")
	} else {
		transport := c.transport
		if transport.MaxIdleConns != 50 {
			t.Errorf("Expected MaxIdleConns to be 50, got %d", transport.MaxIdleConns)
		}
		if transport.IdleConnTimeout != 15*time.Second {
			t.Errorf("Expected IdleConnTimeout to be 15s, got %v", transport.IdleConnTimeout)
		}
	}
	// Test with invalid values
	NewClient().SetMaxIdleConns(0).SetIdleConnTimeout(0)

	// Test client-level hooks
	var beforeHookVal, afterHookVal int
	hookClient := NewClient().
		AddBeforeHook(func(r *http.Request) { beforeHookVal++ }).
		AddAfterHook(func(r *http.Response) { afterHookVal++ })
	hookClient.SetMock(func(req *http.Request) (*http.Response, error) {
		return &http.Response{Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	hookClient.Get(context.Background(), "/hooks")
	if beforeHookVal != 1 {
		t.Errorf("Client-level before hook was not called. Got %d, want 1", beforeHookVal)
	}
	if afterHookVal != 1 {
		t.Errorf("Client-level after hook was not called. Got %d, want 1", afterHookVal)
	}
}

func TestDoRequest(t *testing.T) {
	server := NewMockServer().Handle("/dorequest", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("dorequest-ok"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	req, _ := http.NewRequest("GET", server.URLPrefix+"/dorequest", nil)
	body, err := client.DoRequest(req).GetBody()
	if err != nil {
		t.Fatalf("DoRequest failed: %v", err)
	}
	if string(body) != "dorequest-ok" {
		t.Errorf("Expected body 'dorequest-ok', got %q", string(body))
	}
}

func TestPostForm(t *testing.T) {
	server := NewMockServer().Handle("/form", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		if req.Form.Get("user") != "tester" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("wrong user"))
			return
		}
		if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("wrong content type"))
			return
		}
		w.Write([]byte("form-ok"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	formData := map[string]interface{}{
		"user": "tester",
		"id":   123,
	}
	body, err := client.PostForm(context.Background(), server.URLPrefix+"/form", formData).GetBody()
	if err != nil {
		t.Fatalf("PostForm failed: %v", err)
	}
	if string(body) != "form-ok" {
		t.Errorf("Expected 'form-ok', got %q", string(body))
	}
}

func TestDeleteAndPut(t *testing.T) {
	server := NewMockServer().Handle("/delete", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "DELETE" {
			t.Errorf("Expected DELETE method, got %s", req.Method)
		}
		w.Write([]byte("deleted"))
	}).Handle("/put", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "PUT" {
			t.Errorf("Expected PUT method, got %s", req.Method)
		}
		w.Write([]byte("put-ok"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	resDelete := client.Delete(context.Background(), server.URLPrefix+"/delete", nil)
	if body, _ := resDelete.GetBody(); string(body) != "deleted" {
		t.Errorf("Expected 'deleted', got %q", string(body))
	}

	resPut := client.Put(context.Background(), server.URLPrefix+"/put", nil)
	if body, _ := resPut.GetBody(); string(body) != "put-ok" {
		t.Errorf("Expected 'put-ok', got %q", string(body))
	}
}

func TestPostJSONTypes(t *testing.T) {
	server := NewMockServer().Handle("/json", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	defer server.ServeBackground()()
	client := NewClient()

	// Test with string
	resStr := client.PostJSON(context.Background(), server.URLPrefix+"/json", `{"type":"string"}`)
	if body, _ := resStr.GetBody(); string(body) != `{"type":"string"}` {
		t.Errorf("PostJSON with string failed, got %q", string(body))
	}

	// Test with nil
	resNil := client.PostJSON(context.Background(), server.URLPrefix+"/json", nil)
	if body, _ := resNil.GetBody(); string(body) != "" {
		t.Errorf("PostJSON with nil failed, got body %q", string(body))
	}

	// Test with io.Reader
	reader := strings.NewReader(`{"type":"reader"}`)
	resReader := client.PostJSON(context.Background(), server.URLPrefix+"/json", reader)
	if body, _ := resReader.GetBody(); string(body) != `{"type":"reader"}` {
		t.Errorf("PostJSON with io.Reader failed, got %q", string(body))
	}

	// Test with io.Reader error
	errReader := &errorReader{}
	resErrReader := client.PostJSON(context.Background(), server.URLPrefix+"/json", errReader)
	if resErrReader.Error() == nil {
		t.Error("Expected an error when reading from errorReader, got nil")
	}

	// Test with JSON marshal error
	invalidJSON := make(chan int)
	resInvalid := client.PostJSON(context.Background(), server.URLPrefix+"/json", invalidJSON)
	if resInvalid.Error() == nil {
		t.Error("Expected an error when marshaling invalid JSON, got nil")
	}
	if _, ok := resInvalid.Error().(*json.UnsupportedTypeError); !ok {
		t.Errorf("Expected json.UnsupportedTypeError, got %T", resInvalid.Error())
	}
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestURLRewriter(t *testing.T) {
	RegisterRewriter("testproto", func(ctx context.Context, urlstr string) string {
		return strings.Replace(urlstr, "testproto://", "http://", 1)
	})

	server := NewMockServer().Handle("/rewritten", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("rewritten-ok"))
	})
	defer server.ServeBackground()()

	client := NewClient()
	rewrittenURL := strings.Replace(server.URLPrefix, "http://", "testproto://", 1) + "/rewritten"

	body, err := client.Get(context.Background(), rewrittenURL).GetBody()
	if err != nil {
		t.Fatalf("URL rewriter test failed: %v", err)
	}
	if string(body) != "rewritten-ok" {
		t.Errorf("Expected 'rewritten-ok', got %q", string(body))
	}

	// Test no-op
	resNoOp := client.Get(context.Background(), server.URLPrefix+"/rewritten")
	if resNoOp.Error() != nil {
		t.Fatalf("URL rewriter no-op test failed: %v", resNoOp.Error())
	}
}

func TestSetRetry(t *testing.T) {
	var attemptCount int

	client := NewClient()
	// Mock an endpoint that fails twice before succeeding.
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		attemptCount++
		if attemptCount < 3 {
			return nil, errors.New("transient network error")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("success")),
		}, nil
	})

	// Configure the client to retry up to 2 times.
	client.SetRetry(RetryOption{
		RetryMax:     2,
		RetryWaitMin: 1 * time.Millisecond,
	})

	res := client.Get(context.Background(), "http://test-set-retry")

	if res.Error() != nil {
		t.Fatalf("Expected request to succeed after retries, but got error: %v", res.Error())
	}
	if attemptCount != 3 {
		t.Fatalf("Expected 3 attempts (1 initial + 2 retries), but got %d", attemptCount)
	}
}

func TestWithDialer(t *testing.T) {
	server := NewMockServer().Handle("/dialer", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("dialer-ok"))
	})
	defer server.ServeBackground()()

	dialerCalled := false
	defaultDialer := &net.Dialer{}

	client := NewClient().WithDialer(func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialerCalled = true
		return defaultDialer.DialContext(ctx, network, addr)
	})

	res := client.Get(context.Background(), server.URLPrefix+"/dialer")

	if res.Error() != nil {
		t.Fatalf("Request with custom dialer failed: %v", res.Error())
	}

	if !dialerCalled {
		t.Fatal("Custom dialer was not called")
	}
}

func TestDoWithInvalidURL(t *testing.T) {
	client := NewClient()
	// A URL with a control character is invalid
	invalidURL := "http://invalid-url\x7f.com"
	res := client.Get(context.Background(), invalidURL)
	if res.Error() == nil {
		t.Fatal("Expected an error for invalid URL, but got nil")
	}
	if !strings.Contains(res.Error().Error(), "invalid control character") {
		t.Errorf("Expected error to be about 'invalid control character', got: %v", res.Error())
	}
}

func TestResponseMethods(t *testing.T) {
	// Test Unmarshal with pre-existing error
	errResponse := buildResponse(context.Background(), nil, errors.New("initial error"))
	var data interface{}
	err := errResponse.Unmarshal(&data)
	if err == nil || !strings.Contains(err.Error(), "initial error") {
		t.Errorf("Expected Unmarshal to return the initial error, got: %v", err)
	}

	// Test Unmarshal with malformed JSON
	malformedJSONResponse := &Response{
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader(`{"key": "value`)), // Missing closing brace
		},
	}
	err = malformedJSONResponse.Unmarshal(&data)
	if err == nil {
		t.Fatal("Expected Unmarshal to fail on malformed JSON, but it succeeded")
	}

	// Test Save()
	saveResponse := &Response{
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader("save-test")),
		},
	}
	var buf bytes.Buffer
	err = saveResponse.Save(&buf)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	if buf.String() != "save-test" {
		t.Errorf("Expected saved content to be 'save-test', got %q", buf.String())
	}

	// Test Save() with nil body
	nilBodyResponse := &Response{Response: &http.Response{Body: nil}}
	err = nilBodyResponse.Save(&buf)
	if err != nil {
		t.Errorf("Save() with nil body should not produce an error, got %v", err)
	}
}

type httpRoundingTripper struct{}

func (rt *httpRoundingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func TestPostJSONWithBytes(t *testing.T) {
	server := NewMockServer().Handle("/json-bytes", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		w.Write(body)
	})
	defer server.ServeBackground()()
	client := NewClient()

	payload := []byte(`{"type":"bytes"}`)
	res := client.PostJSON(context.Background(), server.URLPrefix+"/json-bytes", payload)
	if body, _ := res.GetBody(); string(body) != string(payload) {
		t.Errorf("PostJSON with []byte failed, got %q, want %q", string(body), string(payload))
	}
}

func TestStatusCodeCheckEdgeCases(t *testing.T) {
	client := NewClient().AddMiddleware(MiddlewareSetAllowedStatusCode()).AddMiddleware(MiddlewareSetBlockedStatusCode())
	client.SetMock(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound}, nil
	})
	res := client.Get(context.Background(), "/test")
	if res.Error() != nil {
		t.Fatalf("Expected no error when status code checkers are empty, got %v", res.Error())
	}
}

func TestTCPKeepAlive(t *testing.T) {
	server := NewTCPServer()
	server.Start()
	defer server.Stop()

	url := fmt.Sprintf("http://%s/ping", server.addr)
	client := NewClient()
	res, err := client.Get(context.Background(), url).GetBody()
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "PONG" {
		t.Fatalf("bad response `%s`", string(res))
	}
	res, err = client.Get(context.Background(), url).GetBody()
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "PONG" {
		t.Fatalf("bad response `%s`", string(res))
	}
	if server.Connections() != 1 {
		t.Fatalf("keep alive fail %d", server.Connections())
	}
}

func TestTCPKeepAliveWhenUserSetTimeoutContext(t *testing.T) {
	server := NewTCPServer()
	server.Start()
	defer server.Stop()

	url := fmt.Sprintf("http://%s/ping", server.addr)
	client := NewClient()
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*1)
	res, err := client.Get(ctx, url).GetBody()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "PONG" {
		t.Fatalf("bad response `%s`", string(res))
	}
	ctx, cancel = context.WithDeadline(context.TODO(), time.Now().Add(time.Second))
	res, err = client.Get(ctx, url).GetBody()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "PONG" {
		t.Fatalf("bad response `%s`", string(res))
	}
	if server.Connections() != 1 {
		t.Fatalf("keep alive fail %d", server.Connections())
	}
}
