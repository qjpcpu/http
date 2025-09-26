package http

import (
	"bytes"
	"io"
	"net/http"
)

type repeatableReader struct {
	*bytes.Reader
}

func (rr *repeatableReader) SeekStart() error {
	_, err := rr.Seek(0, io.SeekStart)
	return err
}

func (rr *repeatableReader) Close() error {
	return rr.SeekStart()
}

func RepeatableReadResponse(res *http.Response) ([]byte, error) {
	if res == nil || res.Body == nil {
		return nil, nil
	}
	if rr, ok := res.Body.(*repeatableReader); ok {
		defer rr.Close()
		return io.ReadAll(res.Body)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		return nil, err
	}
	res.Body.Close()
	res.Body = &repeatableReader{Reader: bytes.NewReader(data)}
	return data, nil
}

func RepeatableReadRequest(res *http.Request) ([]byte, error) {
	if res.Body == nil {
		return nil, nil
	}
	if rr, ok := res.Body.(*repeatableReader); ok {
		defer rr.Close()
		return io.ReadAll(res.Body)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		res.Body.Close()
		return nil, err
	}
	res.Body.Close()
	res.Body = &repeatableReader{Reader: bytes.NewReader(data)}
	return data, nil
}
