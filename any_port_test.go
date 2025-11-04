package http

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func TestListenOnAnyLocalPort(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	s := ListenOnAnyLocalPort(h)
	go s.Serve()
	time.Sleep(100 * time.Millisecond)
	defer s.Close()

	resp, err := http.Get("http://127.0.0.1" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := ioutil.ReadAll(resp.Body)
	if string(data) != "ok" {
		t.Fatalf("unexpected response: %s", string(data))
	}
}

func TestServeNothing(t *testing.T) {
	s := &ServerOnAnyPort{}
	if err := s.Serve(); err == nil {
		t.Fatal("should return error")
	}
}

func TestListenOnAnyPortError(t *testing.T) {
	s := listenOnAnyPort(nil, "invalid-ip")
	if err := s.Serve(); err == nil {
		t.Fatal("should return error")
	}
}
