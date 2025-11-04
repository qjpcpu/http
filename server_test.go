package http

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	s := NewServer()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.ListenAndServe("tcp", ":8080")
	}()
	time.Sleep(100 * time.Millisecond)
	s.Close(context.Background())
	wg.Wait()
}

func TestServer_Handle(t *testing.T) {
	s := NewServer()
	s.GET("/get", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("get"))
	})
	s.POST("/post", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("post"))
	})
	s.PUT("/put", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("put"))
	})
	s.DELETE("/delete", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("delete"))
	})
	s.Any("/any", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("any"))
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.ListenAndServe("tcp", ":8081")
	}()
	time.Sleep(100 * time.Millisecond)

	t.Run("GET", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8081/get")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		if string(data) != "get" {
			t.Fatalf("unexpected response: %s", string(data))
		}
	})

	t.Run("POST", func(t *testing.T) {
		resp, err := http.Post("http://localhost:8081/post", "text/plain", strings.NewReader("data"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		if string(data) != "post" {
			t.Fatalf("unexpected response: %s", string(data))
		}
	})

	t.Run("PUT", func(t *testing.T) {
		req, _ := http.NewRequest("PUT", "http://localhost:8081/put", strings.NewReader("data"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		if string(data) != "put" {
			t.Fatalf("unexpected response: %s", string(data))
		}
	})

	t.Run("DELETE", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "http://localhost:8081/delete", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		if string(data) != "delete" {
			t.Fatalf("unexpected response: %s", string(data))
		}
	})

	t.Run("ANY", func(t *testing.T) {
		req, _ := http.NewRequest("PATCH", "http://localhost:8081/any", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := ioutil.ReadAll(resp.Body)
		if string(data) != "any" {
			t.Fatalf("unexpected response: %s", string(data))
		}
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		resp, err := http.Post("http://localhost:8081/get", "text/plain", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("unexpected status code: %d", resp.StatusCode)
		}
	})

	s.Close(context.Background())
	wg.Wait()
}

func TestServer_ListenAndServeUnix(t *testing.T) {
	sockFile := "/tmp/test.sock"
	os.Remove(sockFile)

	s := NewServer()
	s.GET("/unix", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("unix"))
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.ListenAndServe("unix", sockFile)
	}()
	time.Sleep(100 * time.Millisecond)

	s.Close(context.Background())
	wg.Wait()
}
