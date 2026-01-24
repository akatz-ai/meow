package ipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// SocketPath returns the IPC socket path for a workflow.
// Format: /tmp/meow-{workflow_id}.sock
func SocketPath(workflowID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("meow-%s.sock", workflowID))
}

// Handler processes IPC messages and returns responses.
// Implementations should be safe for concurrent use.
type Handler interface {
	// HandleStepStart processes a step acknowledgment signal.
	// Called when an agent signals it has received and understood its task.
	// Returns an AckMessage or ErrorMessage.
	HandleStepStart(ctx context.Context, msg *StepStartMessage) any

	// HandleStepDone processes a step completion signal.
	// Returns an AckMessage or ErrorMessage.
	HandleStepDone(ctx context.Context, msg *StepDoneMessage) any

	// HandleGetSessionID returns the Claude session ID for an agent.
	// Returns a SessionIDMessage or ErrorMessage.
	HandleGetSessionID(ctx context.Context, msg *GetSessionIDMessage) any

	// HandleEvent processes an event emitted by an agent.
	// Returns an AckMessage or ErrorMessage.
	HandleEvent(ctx context.Context, msg *EventMessage) any

	// HandleAwaitEvent waits for an event matching the given criteria.
	// Returns an EventMatchMessage or ErrorMessage.
	HandleAwaitEvent(ctx context.Context, msg *AwaitEventMessage) any

	// HandleGetStepStatus returns the status of a step.
	// Returns a StepStatusMessage or ErrorMessage.
	HandleGetStepStatus(ctx context.Context, msg *GetStepStatusMessage) any
}

// Server listens for IPC messages on a Unix domain socket.
type Server struct {
	socketPath string
	handler    Handler
	logger     *slog.Logger

	listener net.Listener
	wg       sync.WaitGroup

	mu       sync.Mutex
	shutdown bool
}

// NewServer creates a new IPC server.
func NewServer(workflowID string, handler Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		socketPath: SocketPath(workflowID),
		handler:    handler,
		logger:     logger.With("component", "ipc-server"),
	}
}

// NewServerWithPath creates a new IPC server with a custom socket path.
// Useful for testing.
func NewServerWithPath(socketPath string, handler Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		socketPath: socketPath,
		handler:    handler,
		logger:     logger.With("component", "ipc-server"),
	}
}

// SocketPath returns the path to the Unix socket.
func (s *Server) Path() string {
	return s.socketPath
}

// Start begins listening for connections.
// This method blocks until the server is shut down.
func (s *Server) Start(ctx context.Context) error {
	// Remove any existing socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create the socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	s.listener = listener

	s.logger.Info("IPC server started", "socket", s.socketPath)

	// Accept connections until shutdown
	go s.acceptLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	return s.Shutdown()
}

// StartAsync starts the server in the background and returns immediately.
// Use Shutdown() to stop the server.
func (s *Server) StartAsync(ctx context.Context) error {
	// Remove any existing socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create the socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	s.listener = listener

	s.logger.Info("IPC server started", "socket", s.socketPath)

	// Accept connections in background
	go s.acceptLoop(ctx)

	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.shutdown = true
	s.mu.Unlock()

	s.logger.Info("IPC server shutting down")

	// Close listener to stop accepting new connections
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.logger.Error("error closing listener", "error", err)
		}
	}

	// Wait for all connections to finish
	s.wg.Wait()

	// Remove socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		s.logger.Error("error removing socket", "error", err)
	}

	s.logger.Info("IPC server stopped")
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			shutdown := s.shutdown
			s.mu.Unlock()

			if shutdown {
				return
			}

			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return
			default:
			}

			s.logger.Error("accept error", "error", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		// Check context
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read one line (one message)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF && !errors.Is(err, net.ErrClosed) {
				s.logger.Error("read error", "error", err)
			}
			return
		}

		// Parse and handle the message
		response := s.handleMessage(ctx, line)

		// Send response
		if err := s.sendResponse(conn, response); err != nil {
			s.logger.Error("write error", "error", err)
			return
		}
	}
}

func (s *Server) handleMessage(ctx context.Context, data []byte) any {
	msg, err := ParseMessage(data)
	if err != nil {
		s.logger.Error("parse error", "error", err, "data", string(data))
		return &ErrorMessage{
			Type:    MsgError,
			Message: fmt.Sprintf("failed to parse message: %v", err),
		}
	}

	switch m := msg.(type) {
	case *StepStartMessage:
		s.logger.Debug("handling step_start", "workflow", m.Workflow, "agent", m.Agent, "step", m.Step)
		return s.handler.HandleStepStart(ctx, m)

	case *StepDoneMessage:
		s.logger.Debug("handling step_done", "workflow", m.Workflow, "agent", m.Agent, "step", m.Step)
		return s.handler.HandleStepDone(ctx, m)

	case *GetSessionIDMessage:
		s.logger.Debug("handling get_session_id", "agent", m.Agent)
		return s.handler.HandleGetSessionID(ctx, m)

	case *EventMessage:
		s.logger.Debug("handling event", "event_type", m.EventType, "agent", m.Agent)
		return s.handler.HandleEvent(ctx, m)

	case *AwaitEventMessage:
		s.logger.Debug("handling await_event", "event_type", m.EventType, "timeout", m.Timeout)
		return s.handler.HandleAwaitEvent(ctx, m)

	case *GetStepStatusMessage:
		s.logger.Debug("handling get_step_status", "workflow", m.Workflow, "step_id", m.StepID)
		return s.handler.HandleGetStepStatus(ctx, m)

	default:
		s.logger.Error("unexpected message type", "type", fmt.Sprintf("%T", msg))
		return &ErrorMessage{
			Type:    MsgError,
			Message: fmt.Sprintf("unexpected message type: %T", msg),
		}
	}
}

func (s *Server) sendResponse(conn net.Conn, response any) error {
	data, err := Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Add newline delimiter
	data = append(data, '\n')

	_, err = conn.Write(data)
	return err
}
