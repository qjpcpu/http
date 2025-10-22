package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
)

const anyMethod = "*"

type Server struct {
	server   *http.Server
	mux      *http.ServeMux
	handlers sync.Map
}

type ServerOption func(*http.Server)

func NewServer() *Server {
	s := &Server{
		mux:    http.NewServeMux(),
		server: &http.Server{},
	}
	return s
}

func (s *Server) GET(pattern string, h http.HandlerFunc) {
	s.Handle("GET", pattern, h)
}

func (s *Server) POST(pattern string, h http.HandlerFunc) {
	s.Handle("POST", pattern, h)
}

func (s *Server) PUT(pattern string, h http.HandlerFunc) {
	s.Handle("PUT", pattern, h)
}

func (s *Server) DELETE(pattern string, h http.HandlerFunc) {
	s.Handle("DELETE", pattern, h)
}

func (s *Server) Any(pattern string, h http.HandlerFunc) {
	s.Handle(anyMethod, pattern, h)
}

func (s *Server) ListenAndServe(network, addr string, opts ...ServerOption) error {
	var ln net.Listener
	switch network {
	case "unix":
		sock, err := filepath.Abs(addr)
		if err != nil {
			return err
		}
		dir := filepath.Dir(sock)
		if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
		os.RemoveAll(sock)
		unixAddr, err := net.ResolveUnixAddr("unix", sock)
		if err != nil {
			return err
		}
		ln, err = net.ListenUnix("unix", unixAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
	case "tcp":
		var err error
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
	default:
		return fmt.Errorf("not support network %s", network)
	}
	return s.Serve(ln, opts...)
}

func (s *Server) Serve(ln net.Listener, opts ...ServerOption) error {
	for _, fn := range opts {
		fn(s.server)
	}
	s.server.Handler = s.mux
	return s.server.Serve(ln)
}

func (s *Server) Close(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Handle(method, pattern string, h http.HandlerFunc) {
	actual, loaded := s.handlers.LoadOrStore(pattern, new(sync.Map))
	mh := actual.(*sync.Map)
	mh.Store(strings.ToUpper(method), h)
	if !loaded {
		s.mux.HandleFunc(pattern, s.makeHandler(mh))
	}
}

func (s *Server) makeHandler(hs *sync.Map) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				http.Error(w, fmt.Sprintf("%v\n%s", p, debug.Stack()), http.StatusInternalServerError)
			}
		}()
		if val, ok := hs.Load(r.Method); ok {
			val.(http.HandlerFunc).ServeHTTP(w, r)
			return
		}
		if val, ok := hs.Load(anyMethod); ok {
			val.(http.HandlerFunc).ServeHTTP(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
