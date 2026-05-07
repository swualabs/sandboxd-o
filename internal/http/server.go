package httpserver

import (
	"net/http"

	"example.com/sandbox-demo/internal/sandbox"
)

type Server struct {
	svc   *sandbox.Service
	ipSvc externalIPService
}

func New(svc *sandbox.Service) *Server {
	return &Server{svc: svc, ipSvc: commandExternalIPService{}}
}

func (s *Server) Handler() http.Handler {
	return newRouter(s)
}
