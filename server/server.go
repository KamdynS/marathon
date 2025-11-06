// Package server provides HTTP API for workflow management and monitoring.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/KamdynS/marathon/engine"
	"github.com/KamdynS/marathon/server/agenthttp"
)

// Server provides HTTP API for workflows
type Server struct {
	engine     *engine.Engine
	httpServer *http.Server
	port       int
}

// Config holds server configuration
type Config struct {
	Engine *engine.Engine
	Port   int
}

// New creates a new workflow API server
func New(cfg Config) (*Server, error) {
	if cfg.Engine == nil {
		return nil, fmt.Errorf("engine is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	server := &Server{
		engine: cfg.Engine,
		port:   cfg.Port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/workflows", server.handleWorkflows)
	mux.HandleFunc("/workflows/", server.handleWorkflowByID)
	mux.HandleFunc("/health", server.handleHealth)

	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	log.Printf("[Server] Starting workflow API server on port %d", s.port)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	log.Printf("[Server] Stopping workflow API server")
	return s.httpServer.Shutdown(ctx)
}

// StartWorkflowRequest represents a request to start a workflow
type StartWorkflowRequest struct {
	WorkflowName string      `json:"workflow_name"`
	Input        interface{} `json:"input"`
}

// StartWorkflowResponse represents a response from starting a workflow
type StartWorkflowResponse struct {
	WorkflowID string `json:"workflow_id"`
}

// WorkflowStatusResponse represents a workflow status response
type WorkflowStatusResponse struct {
	WorkflowID   string      `json:"workflow_id"`
	WorkflowName string      `json:"workflow_name"`
	Status       string      `json:"status"`
	Input        interface{} `json:"input"`
	Output       interface{} `json:"output,omitempty"`
	Error        string      `json:"error,omitempty"`
	StartTime    time.Time   `json:"start_time"`
	EndTime      *time.Time  `json:"end_time,omitempty"`
	Duration     string      `json:"duration"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// handleWorkflows handles POST /workflows (start workflow)
func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req StartWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.WorkflowName == "" {
		s.sendError(w, http.StatusBadRequest, "workflow_name is required")
		return
	}

    idemKey := r.Header.Get("Idempotency-Key")
    workflowID, err := s.engine.StartWorkflowWithOptions(r.Context(), req.WorkflowName, req.Input, engine.StartWorkflowOptions{IdempotencyKey: idemKey})
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start workflow: %v", err))
		return
	}

	resp := StartWorkflowResponse{
		WorkflowID: workflowID,
	}

	s.sendJSON(w, http.StatusOK, resp)
}

// handleWorkflowByID handles GET /workflows/{id}, GET /workflows/{id}/events, POST /workflows/{id}/cancel
func (s *Server) handleWorkflowByID(w http.ResponseWriter, r *http.Request) {
	// Extract workflow ID from path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 2 {
		s.sendError(w, http.StatusNotFound, "workflow ID required")
		return
	}

	workflowID := pathParts[1]

	// Route based on path and method
	if len(pathParts) == 2 {
		// /workflows/{id}
		switch r.Method {
		case http.MethodGet:
			s.handleGetWorkflowStatus(w, r, workflowID)
		default:
			s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	} else if len(pathParts) == 3 {
		// /workflows/{id}/{action}
		action := pathParts[2]
		switch action {
		case "events":
			if r.Method == http.MethodGet {
				s.handleGetWorkflowEvents(w, r, workflowID)
			} else {
				s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		case "cancel":
			if r.Method == http.MethodPost {
				s.handleCancelWorkflow(w, r, workflowID)
			} else {
				s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		default:
			s.sendError(w, http.StatusNotFound, "unknown action")
		}
	} else {
		s.sendError(w, http.StatusNotFound, "not found")
	}
}

// handleGetWorkflowStatus handles GET /workflows/{id}
func (s *Server) handleGetWorkflowStatus(w http.ResponseWriter, r *http.Request, workflowID string) {
	workflowState, err := s.engine.GetWorkflowStatus(r.Context(), workflowID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, fmt.Sprintf("workflow not found: %v", err))
		return
	}

	resp := WorkflowStatusResponse{
		WorkflowID:   workflowState.WorkflowID,
		WorkflowName: workflowState.WorkflowName,
		Status:       string(workflowState.Status),
		Input:        workflowState.Input,
		Output:       workflowState.Output,
		Error:        workflowState.Error,
		StartTime:    workflowState.StartTime,
		EndTime:      workflowState.EndTime,
		Duration:     workflowState.Duration().String(),
	}

	s.sendJSON(w, http.StatusOK, resp)
}

// handleGetWorkflowEvents handles GET /workflows/{id}/events
func (s *Server) handleGetWorkflowEvents(w http.ResponseWriter, r *http.Request, workflowID string) {
	// Check if SSE is requested
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/event-stream") {
		s.handleWorkflowEventsSSE(w, r, workflowID)
		return
	}

	// Return all events as JSON
	events, err := s.engine.GetWorkflowEvents(r.Context(), workflowID)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get events: %v", err))
		return
	}

	s.sendJSON(w, http.StatusOK, events)
}

// handleWorkflowEventsSSE handles SSE streaming of workflow events
func (s *Server) handleWorkflowEventsSSE(w http.ResponseWriter, r *http.Request, workflowID string) {
	lastID := r.Header.Get("Last-Event-ID")
	_ = agenthttp.StreamEvents(
		r.Context(),
		w,
		lastID,
		s.engine.GetWorkflowEventsSince,
		workflowID,
		500*time.Millisecond,
		15*time.Second,
	)
}

// handleCancelWorkflow handles POST /workflows/{id}/cancel
func (s *Server) handleCancelWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) {
	if err := s.engine.CancelWorkflow(r.Context(), workflowID); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("failed to cancel workflow: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// sendJSON sends a JSON response
func (s *Server) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sendError sends an error response
func (s *Server) sendError(w http.ResponseWriter, status int, message string) {
	s.sendJSON(w, status, ErrorResponse{Error: message})
}
