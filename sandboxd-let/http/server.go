package httpserver

import (
	"net/http"

	"sandboxd-o/pkg/logging"
	"sandboxd-o/sandboxd-let/sandbox"
)

type Server struct {
	svc          *sandbox.Service
	log          *logging.Logger
	sharedSecret string
}

func New(svc *sandbox.Service, logger *logging.Logger, sharedSecret string) *Server {
	return &Server{svc: svc, log: logger, sharedSecret: sharedSecret}
}

func (s *Server) Handler() http.Handler {
	return newRouter(s)
}
