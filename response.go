package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Request struct {
	*http.Request
}

func FromRequest(req *http.Request) *Request {
	return &Request{Request: req}
}

func (req *Request) AddRetryHook(hook RetryHook) {
	getValue(req.Request).AddRetryHook(hook)
}

type Response struct {
	*http.Response
	err  error
	ctx  context.Context
	read int32
}

type ResponseHandler func(*http.Response) error

// HandleResult is the core method for processing the HTTP response. It ensures that the
// response body is read and closed only once.
//
// NOTE: This method, and any method that calls it (like Unmarshal, GetBody, Save),
// consumes the response body. It should only be called once per Response object.
func (r *Response) HandleResult(f ResponseHandler) error {
	if r.read == 0 {
		r.read = 1
		if r.Response != nil {
			if r.Response.Body != nil {
				defer r.Response.Body.Close()
			}
			if r.err == nil && f != nil {
				r.err = f(r.Response)
			}
		}
	}
	return r.err
}

// Error returns the error, if any, that occurred during the request.
// It also ensures the response body is fully read and closed, which is crucial for
// connection reuse.
//
// NOTE: This method consumes the response body. Do not call other body-processing
// methods (like Unmarshal, GetBody) after calling Error.
func (r *Response) Error() error {
	return r.Save(nil)
}

// Unmarshal parses the JSON-encoded response body and stores the result in the
// value pointed to by obj.
//
// NOTE: This method consumes the response body and can only be called once.
func (r *Response) Unmarshal(obj any) error {
	return r.HandleResult(func(res *http.Response) error {
		var requrl, resCode string
		if r.Response != nil {
			resCode = r.Response.Status
			if r.Response != nil && r.Response.Request != nil && r.Response.Request.URL != nil {
				requrl = r.Response.Request.URL.String()
			}
		}
		if res.Body == nil {
			return nil
		}
		data, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("get response body fail %v, url=%s response_code=%s %w", err, requrl, resCode, err)
		}
		if obj != nil {
			if err = json.Unmarshal(data, obj); err != nil {
				return fmt.Errorf("unmarshal body %s fail %v, uri=%s respons_code=%s %w", string(data), err, requrl, resCode, err)
			}
		}
		return nil
	})
}

// GetBody reads and returns the entire response body as a byte slice.
//
// NOTE: This method consumes the response body and can only be called once.
func (r *Response) GetBody() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := r.Save(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *Response) MustGetBody() []byte {
	data, err := r.GetBody()
	if err != nil {
		panic(err)
	}
	return data
}

// Save reads the entire response body and writes it to the provided io.Writer.
// If the writer is nil, the body is read and discarded.
//
// NOTE: This method consumes the response body and can only be called once.
func (r *Response) Save(w io.Writer) error {
	return r.HandleResult(func(res *http.Response) error {
		if w == nil {
			w = io.Discard
		}
		if res.Body != nil {
			_, err := io.Copy(w, r.Response.Body)
			return err
		}
		return nil
	})
}

func buildResponse(ctx context.Context, res *http.Response, err error) *Response {
	if res == nil {
		res = &http.Response{}
	}
	return &Response{ctx: ctx, Response: res, err: err}
}
