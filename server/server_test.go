package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KamdynS/marathon/engine"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/workflow"
)

func setupTestServer(t *testing.T) *Server {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	workflowRegistry := workflow.NewRegistry()

	// Register a test workflow
	def := workflow.New("test-workflow").Build()
	if err := workflowRegistry.Register(def); err != nil {
		t.Fatalf("failed to register workflow: %v", err)
	}

	eng, err := engine.New(engine.Config{
		StateStore:       store,
		Queue:            q,
		WorkflowRegistry: workflowRegistry,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	server, err := New(Config{
		Engine: eng,
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	return server
}

func TestServer_StartWorkflow(t *testing.T) {
	server := setupTestServer(t)

	reqBody := StartWorkflowRequest{
		WorkflowName: "test-workflow",
		Input:        map[string]string{"key": "value"},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleWorkflows(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp StartWorkflowResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.WorkflowID == "" {
		t.Error("expected workflow ID to be returned")
	}
}

func TestServer_GetWorkflowStatus(t *testing.T) {
	server := setupTestServer(t)

	// Start a workflow first
	ctx := context.Background()
	workflowID, err := server.engine.StartWorkflow(ctx, "test-workflow", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Get status
	req := httptest.NewRequest(http.MethodGet, "/workflows/"+workflowID, nil)
	w := httptest.NewRecorder()

	server.handleWorkflowByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp WorkflowStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.WorkflowID != workflowID {
		t.Errorf("expected workflow ID %s, got %s", workflowID, resp.WorkflowID)
	}

	if resp.WorkflowName != "test-workflow" {
		t.Errorf("expected workflow name test-workflow, got %s", resp.WorkflowName)
	}
}

func TestServer_GetWorkflowEvents(t *testing.T) {
	server := setupTestServer(t)

	// Start a workflow first
	ctx := context.Background()
	workflowID, err := server.engine.StartWorkflow(ctx, "test-workflow", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Get events
	req := httptest.NewRequest(http.MethodGet, "/workflows/"+workflowID+"/events", nil)
	w := httptest.NewRecorder()

	server.handleWorkflowByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var events []*state.Event
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(events) < 1 {
		t.Error("expected at least 1 event")
	}

	if events[0].Type != state.EventWorkflowStarted {
		t.Errorf("expected first event to be workflow started, got %s", events[0].Type)
	}
}

func TestServer_CancelWorkflow(t *testing.T) {
	server := setupTestServer(t)

	// Start a workflow first
	ctx := context.Background()
	workflowID, err := server.engine.StartWorkflow(ctx, "test-workflow", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	// Cancel workflow
	req := httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID+"/cancel", nil)
	w := httptest.NewRecorder()

	server.handleWorkflowByID(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify workflow is canceled
	workflowState, err := server.engine.GetWorkflowStatus(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if workflowState.Status != state.StatusCanceled {
		t.Errorf("expected status canceled, got %s", workflowState.Status)
	}
}

func TestServer_Health(t *testing.T) {
	server := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestServer_InvalidRequests(t *testing.T) {
	server := setupTestServer(t)

	tests := []struct {
		name           string
		method         string
		path           string
		body           []byte
		expectedStatus int
	}{
		{
			name:           "missing workflow name",
			method:         http.MethodPost,
			path:           "/workflows",
			body:           []byte(`{"input": "test"}`),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			path:           "/workflows",
			body:           []byte(`{invalid}`),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "workflow not found",
			method:         http.MethodGet,
			path:           "/workflows/non-existent",
			body:           nil,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "method not allowed",
			method:         http.MethodDelete,
			path:           "/workflows",
			body:           nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != nil {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			w := httptest.NewRecorder()

			if tt.path == "/workflows" {
				server.handleWorkflows(w, req)
			} else {
				server.handleWorkflowByID(w, req)
			}

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
