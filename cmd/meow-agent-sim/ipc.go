package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

// IPCClient is a real Unix socket IPC client for orchestrator communication.
type IPCClient struct {
	socketPath string
	agentID    string
	workflowID string
	stepID     string

	mu   sync.Mutex
	conn net.Conn
}

// NewIPCClient creates a new IPC client.
func NewIPCClient(socketPath string) *IPCClient {
	return &IPCClient{
		socketPath: socketPath,
		agentID:    os.Getenv("MEOW_AGENT"),
		workflowID: os.Getenv("MEOW_WORKFLOW"),
		stepID:     os.Getenv("MEOW_STEP"),
	}
}

// connect establishes a connection to the orchestrator socket.
func (c *IPCClient) connect() error {
	if c.conn != nil {
		return nil
	}

	if c.socketPath == "" {
		return fmt.Errorf("MEOW_ORCH_SOCK not set")
	}

	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("connecting to orchestrator: %w", err)
	}

	c.conn = conn
	return nil
}

// sendAndReceive sends a message and reads the response.
func (c *IPCClient) sendAndReceive(msg any) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.connect(); err != nil {
		return nil, err
	}

	// Marshal and send
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshaling message: %w", err)
	}

	if _, err := c.conn.Write(append(data, '\n')); err != nil {
		c.conn.Close()
		c.conn = nil
		return nil, fmt.Errorf("sending message: %w", err)
	}

	// Read response
	reader := bufio.NewReader(c.conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		c.conn.Close()
		c.conn = nil
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return line, nil
}

// sendFireAndForget sends a message without waiting for response.
func (c *IPCClient) sendFireAndForget(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.connect(); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	if _, err := c.conn.Write(append(data, '\n')); err != nil {
		c.conn.Close()
		c.conn = nil
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

// StepDone signals step completion to the orchestrator.
func (c *IPCClient) StepDone(outputs map[string]any) error {
	msg := map[string]any{
		"type":     "step_done",
		"workflow": c.workflowID,
		"agent":    c.agentID,
		"step":     c.stepID,
		"outputs":  outputs,
	}

	resp, err := c.sendAndReceive(msg)
	if err != nil {
		return err
	}

	// Parse response to check for errors
	var result struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if result.Type == "error" {
		return fmt.Errorf("orchestrator error: %s", result.Message)
	}

	return nil
}

// GetPrompt retrieves the current prompt from the orchestrator.
func (c *IPCClient) GetPrompt() (string, error) {
	msg := map[string]any{
		"type":  "get_prompt",
		"agent": c.agentID,
	}

	resp, err := c.sendAndReceive(msg)
	if err != nil {
		return "", err
	}

	var result struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.Type == "error" {
		return "", fmt.Errorf("orchestrator error: %s", result.Message)
	}

	return result.Content, nil
}

// Event sends an event to the orchestrator (fire-and-forget).
func (c *IPCClient) Event(eventType string, data map[string]any) error {
	msg := map[string]any{
		"type":       "event",
		"event_type": eventType,
		"data":       data,
		"agent":      c.agentID,
		"workflow":   c.workflowID,
	}

	return c.sendFireAndForget(msg)
}

// Close closes the IPC connection.
func (c *IPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}
