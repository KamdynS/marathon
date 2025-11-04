package agenthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/KamdynS/marathon/agent"
)

// Server wraps an agent with HTTP endpoints for chat and streaming.
type Server struct {
	agent  agent.Agent
	config Config
	http   *http.Server
}

// Config for the HTTP server.
type Config struct {
	Port                int
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	RequestTimeout      time.Duration
	MaxRequestBodyBytes int64
}

// New constructs the server.
func New(a agent.Agent, cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 60 * time.Second
	}
	if cfg.MaxRequestBodyBytes == 0 {
		cfg.MaxRequestBodyBytes = 1 << 20
	}

	s := &Server{agent: a, config: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/chat", s.chat)
	mux.HandleFunc("/chat/stream", s.stream)

	s.http = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      s.wrap(mux),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return s
}

func (s *Server) wrap(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, s.config.RequestTimeout, "request timeout")
}

// Start the HTTP server.
func (s *Server) Start() error { return s.http.ListenAndServe() }

// Stop the HTTP server.
func (s *Server) Stop(ctx context.Context) error { return s.http.Shutdown(ctx) }

type ChatRequest struct {
	Message   string            `json:"message"`
	SessionID string            `json:"session_id,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type ChatResponse struct {
	Message   string            `json:"message"`
	SessionID string            `json:"session_id,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	Error     string            `json:"error,omitempty"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req ChatRequest
	if err := dec.Decode(&req); err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Message == "" {
		s.writeErr(w, http.StatusBadRequest, "message is required")
		return
	}
	input := agent.Message{Role: "user", Content: req.Message, Meta: req.Meta}
	resp, err := s.agent.Run(r.Context(), input)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ChatResponse{Message: resp.Content, SessionID: req.SessionID, Meta: resp.Meta})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req ChatRequest
	if err := dec.Decode(&req); err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	input := agent.Message{Role: "user", Content: req.Message, Meta: req.Meta}
	out := make(chan agent.Message)
	go func() { _ = s.agent.RunStream(r.Context(), input, out) }()
	for {
		select {
		case m, ok := <-out:
			if !ok {
				fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			data, _ := json.Marshal(ChatResponse{Message: m.Content, SessionID: req.SessionID, Meta: m.Meta})
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
	}
}

func (s *Server) writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ChatResponse{Error: msg})
}
