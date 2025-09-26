package http

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type MockServer struct {
	mux       *http.ServeMux
	server    *ServerOnAnyPort
	URLPrefix string
}

func NewMockServer() *MockServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", Echo)
	return &MockServer{mux: mux}
}

func (ms *MockServer) Handle(path string, fn func(w http.ResponseWriter, req *http.Request)) *MockServer {
	ms.mux.HandleFunc(path, fn)
	return ms
}

func (ms *MockServer) ServeBackground() func() {
	ms.server = ListenOnAnyPort(ms.mux)
	go ms.server.Serve()
	ms.URLPrefix = "http://127.0.0.1" + ms.server.Addr()
	return func() {
		ms.server.Close()
	}
}

func Echo(w http.ResponseWriter, req *http.Request) {
	args := make(map[string]string)
	qs := req.URL.Query()
	for k := range qs {
		args[k] = qs.Get(k)
	}

	header := make(map[string]string)
	for k := range req.Header {
		header[k] = req.Header.Get(k)
	}

	body, _ := io.ReadAll(req.Body)

	output, _ := json.Marshal(map[string]interface{}{
		"args":    args,
		"headers": header,
		"body":    string(body),
		"url":     req.URL.String(),
	})
	w.Write(output)
}

type TCPServer struct {
	listener    net.Listener
	connections int64
	addr        string
}

func NewTCPServer() *TCPServer {
	return &TCPServer{}
}

func (s *TCPServer) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", s.addr, err)
	}
	s.addr = fmt.Sprintf("localhost:%d", s.listener.Addr().(*net.TCPAddr).Port)
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				return
			}

			if tcpConn, ok := conn.(*net.TCPConn); ok {
				tcpConn.SetKeepAlive(true)
				tcpConn.SetKeepAlivePeriod(30 * time.Second)
			}

			atomic.AddInt64(&s.connections, 1)

			go s.handleConnection(conn)
		}
	}()
	return nil
}

func (s *TCPServer) Connections() int64 {
	return atomic.LoadInt64(&s.connections)
}

func (s *TCPServer) handleConnection(conn net.Conn) {
	// Use a bufio.Reader for efficient, buffered I/O.
	reader := bufio.NewReader(conn)
	defer conn.Close()

	for {
		// Set a deadline for reading the next request to avoid hanging on idle connections.
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Use http.ReadRequest to reliably parse a full HTTP request.
		req, err := http.ReadRequest(reader)
		if err != nil {
			// io.EOF means the client has closed the connection gracefully.
			if err != io.EOF {
				fmt.Printf("TCPServer: error reading request: %v\n", err)
			}
			return
		}

		// Always drain and close the request body.
		io.Copy(io.Discard, req.Body)
		req.Body.Close()

		s.handlePing(conn)
	}
}

func (s *TCPServer) handlePing(conn net.Conn) {
	// Use http.Response.Write to generate a valid HTTP response.
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(strings.NewReader("PONG")),
		ContentLength: 4,
	}
	resp.Write(conn)
}

func (s *TCPServer) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
