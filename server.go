package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
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
		mux: http.NewServeMux(),
		server: &http.Server{
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
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

func (s *Server) ListenAndServe(addr string, opts ...ServerOption) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
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

func (s *Server) Close() error {
	return s.server.Shutdown(context.Background())
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
