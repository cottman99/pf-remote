package localapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/cottman99/pf-remote/internal/actions"
)

type Server struct {
	Handler Handler
	wg      sync.WaitGroup
}

func NewServer() *Server { return &Server{Handler: NewHandler()} }

func NewServerWithService(service actions.Service) *Server {
	return &Server{Handler: Handler{Service: service}}
}

func NewServerWithProvider(provider ServiceProvider) *Server {
	return &Server{Handler: Handler{Provider: provider}}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	defer s.wg.Wait()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer connection.Close()
			s.serveConnection(ctx, connection)
		}()
	}
}

func (s *Server) serveConnection(ctx context.Context, connection net.Conn) {
	decoder := json.NewDecoder(bufio.NewReader(io.LimitReader(connection, 1<<20)))
	encoder := json.NewEncoder(connection)
	var request Request
	if err := decoder.Decode(&request); err != nil {
		_ = encoder.Encode(failed("INVALID_REQUEST", "local-api", "The local request is invalid.", "Send one JSON request smaller than 1 MiB."))
		return
	}
	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, connection)
		cancel()
	}()
	_ = encoder.Encode(s.Handler.HandleContext(requestContext, request))
}
