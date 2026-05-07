package httpserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"example.com/sandbox-demo/internal/model"
	"github.com/gin-gonic/gin"
)

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) createSandbox(c *gin.Context) {
	var req model.CreateSandboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if req.ID == "" {
		req.ID = "sbx-" + time.Now().UTC().Format("20060102-150405")
	}

	opCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	sbx, err := s.svc.CreateSandboxAsync(opCtx, req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	respondJSON(c, http.StatusAccepted, gin.H{"sandbox": sbx}, s.ipSvc.Lookup(c.Request.Context()))
}

func (s *Server) getSandbox(c *gin.Context) {
	sbx, err := s.svc.GetSandbox(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondErrorMessage(c, http.StatusNotFound, "not found")
			return
		}

		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondJSON(c, http.StatusOK, gin.H{"sandbox": sbx}, s.ipSvc.Lookup(c.Request.Context()))
}

func (s *Server) listSandboxes(c *gin.Context) {
	items, err := s.svc.ListSandboxes(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondJSON(c, http.StatusOK, gin.H{"items": items}, s.ipSvc.Lookup(c.Request.Context()))
}

func (s *Server) deleteSandbox(c *gin.Context) {
	opCtx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	if err := s.svc.DeleteSandbox(opCtx, c.Param("id")); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondJSON(c, http.StatusOK, gin.H{"id": c.Param("id"), "phase": "deleted"}, s.ipSvc.Lookup(c.Request.Context()))
}

func (s *Server) reconcile(c *gin.Context) {
	opCtx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	if err := s.svc.ReconcileOnce(opCtx); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondJSON(c, http.StatusOK, gin.H{"ok": true}, s.ipSvc.Lookup(c.Request.Context()))
}
