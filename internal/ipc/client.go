package ipc

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// Client connects to an IPC server to send messages.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a new IPC client.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    30 * time.Second,
	}
}

// NewClientForWorkflow creates a client for a specific workflow's IPC socket.
func NewClientForWorkflow(workflowID string) *Client {
	return NewClient(SocketPath(workflowID))
}

// SetTimeout sets the connection and read/write timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// Send sends a message and waits for a response.
// The response is parsed and returned as the appropriate message type.
func (c *Client) Send(msg any) (any, error) {
	// Connect to socket
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IPC socket %s: %w", c.socketPath, err)
	}
	defer conn.Close()

	// Set deadline for the entire operation
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	// Marshal and send message
	data, err := Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Add newline delimiter
	data = append(data, '\n')

	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	responseLine, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	response, err := ParseMessage(responseLine)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response, nil
}

// SendStepDone sends a step completion message.
// Returns the parsed response (AckMessage or ErrorMessage).
func (c *Client) SendStepDone(workflow, agent, step string, outputs map[string]any, notes string) (any, error) {
	msg := &StepDoneMessage{
		Type:     MsgStepDone,
		Workflow: workflow,
		Agent:    agent,
		Step:     step,
		Outputs:  outputs,
		Notes:    notes,
	}
	return c.Send(msg)
}

// GetPrompt requests the current prompt for an agent.
// Returns the prompt content (empty string means "stay idle").
func (c *Client) GetPrompt(agent string) (string, error) {
	msg := &GetPromptMessage{
		Type:  MsgGetPrompt,
		Agent: agent,
	}

	response, err := c.Send(msg)
	if err != nil {
		return "", err
	}

	switch r := response.(type) {
	case *PromptMessage:
		return r.Content, nil
	case *ErrorMessage:
		return "", fmt.Errorf("server error: %s", r.Message)
	default:
		return "", fmt.Errorf("unexpected response type: %T", response)
	}
}

// GetSessionID requests the Claude session ID for an agent.
func (c *Client) GetSessionID(agent string) (string, error) {
	msg := &GetSessionIDMessage{
		Type:  MsgGetSessionID,
		Agent: agent,
	}

	response, err := c.Send(msg)
	if err != nil {
		return "", err
	}

	switch r := response.(type) {
	case *SessionIDMessage:
		return r.SessionID, nil
	case *ErrorMessage:
		return "", fmt.Errorf("server error: %s", r.Message)
	default:
		return "", fmt.Errorf("unexpected response type: %T", response)
	}
}

// SendApproval sends an approval or rejection for a gate.
func (c *Client) SendApproval(workflow, gateID string, approved bool, notes, reason string) error {
	msg := &ApprovalMessage{
		Type:     MsgApproval,
		Workflow: workflow,
		GateID:   gateID,
		Approved: approved,
		Notes:    notes,
		Reason:   reason,
	}

	response, err := c.Send(msg)
	if err != nil {
		return err
	}

	switch r := response.(type) {
	case *AckMessage:
		if !r.Success {
			return fmt.Errorf("approval was not successful")
		}
		return nil
	case *ErrorMessage:
		return fmt.Errorf("server error: %s", r.Message)
	default:
		return fmt.Errorf("unexpected response type: %T", response)
	}
}
