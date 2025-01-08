package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	syshttp "net/http"
)

type Request struct {
	*syshttp.Request
}

func FromRequest(req *syshttp.Request) *Request {
	return &Request{Request: req}
}

func (req *Request) AddRetryHook(hook RetryHook) {
	getValue(req.Request).AddRetryHook(hook)
}

type Response struct {
	*syshttp.Response
	Err error
	ctx context.Context
}

func (r *Response) Result() (*syshttp.Response, error) {
	return r.Response, r.Err
}

func (r *Response) Unmarshal(obj interface{}) error {
	ctx := r.ctx
	if r.Err != nil {
		log(ctx, "http response error %v", r.Err)
		return r.Err
	}
	var requrl, resCode string
	if r.Response != nil {
		resCode = r.Response.Status
		if r.Response != nil && r.Response.Request != nil && r.Response.Request.URL != nil {
			requrl = r.Response.Request.URL.String()
		}
	}
	data, err := r.GetBody()
	if err != nil {
		log(ctx, "get response body fail %v, url=%s response_code=%s", err, requrl, resCode)
		return err
	}
	if obj != nil {
		if err = json.Unmarshal(data, obj); err != nil {
			log(ctx, "unmarshal body %s fail %v, uri=%s respons_code=%s", string(data), err, requrl, resCode)
		}
	}
	return err
}

func (r *Response) MustGetBody() []byte {
	data, err := r.GetBody()
	if err != nil {
		panic(err)
	}
	return data
}

func (r *Response) GetBody() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := r.Save(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *Response) Save(w io.Writer) error {
	if r.Err != nil {
		return r.Err
	}
	if r.Response == nil || r.Response.Body == nil {
		return nil
	}
	defer r.Response.Body.Close()
	_, err := io.Copy(w, r.Response.Body)
	return err
}

func buildResponse(ctx context.Context, res *syshttp.Response, err error) *Response {
	if res == nil {
		res = &syshttp.Response{}
	}
	return &Response{ctx: ctx, Response: res, Err: err}
}
