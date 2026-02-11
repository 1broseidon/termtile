package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/1broseidon/termtile/internal/runtimepath"
)

// Client handles IPC communication with the daemon
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a new IPC client
func NewClient() *Client {
	socketPath, err := runtimepath.SocketPath()
	if err != nil {
		// Keep constructor non-failing; sendRequest surfaces connection errors.
		socketPath = ""
	}

	return &Client{
		socketPath: socketPath,
		timeout:    5 * time.Second,
	}
}

// sendRequest sends a request and waits for a response
func (c *Client) sendRequest(req *Request) (*Response, error) {
	// Connect to socket
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w (is the daemon running?)", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(c.timeout))

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	reqData = append(reqData, '\n')
	if _, err := conn.Write(reqData); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	respData, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for error response
	if resp.Status == "ERROR" {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	return &resp, nil
}

// Reload sends a RELOAD command to the daemon
func (c *Client) Reload() error {
	req := &Request{
		Command: CommandReload,
	}

	_, err := c.sendRequest(req)
	return err
}

// Undo sends an UNDO command to the daemon.
func (c *Client) Undo() error {
	req := &Request{
		Command: CommandUndo,
	}

	_, err := c.sendRequest(req)
	return err
}

// GetStatus retrieves daemon status
func (c *Client) GetStatus() (*StatusData, error) {
	req := &Request{
		Command: CommandGetStatus,
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var status StatusData
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status data: %w", err)
	}

	return &status, nil
}

// GetMonitors retrieves monitor information
func (c *Client) GetMonitors() (*MonitorsData, error) {
	req := &Request{
		Command: CommandGetMonitors,
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var monitors MonitorsData
	if err := json.Unmarshal(resp.Data, &monitors); err != nil {
		return nil, fmt.Errorf("failed to parse monitors data: %w", err)
	}

	return &monitors, nil
}

// PreviewLayout temporarily applies a layout for preview
func (c *Client) PreviewLayout(layoutName string, durationSeconds int) error {
	payload, err := json.Marshal(PreviewLayoutPayload{
		LayoutName:      layoutName,
		DurationSeconds: durationSeconds,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal preview payload: %w", err)
	}

	req := &Request{
		Command: CommandPreviewLayout,
		Payload: payload,
	}

	_, err = c.sendRequest(req)
	return err
}

// ListLayouts retrieves available layouts and current selection.
func (c *Client) ListLayouts() (*LayoutsData, error) {
	req := &Request{
		Command: CommandListLayouts,
	}

	resp, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var data LayoutsData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse layouts data: %w", err)
	}

	return &data, nil
}

// ApplyLayout sets the daemon's active layout (optionally tiles immediately).
func (c *Client) ApplyLayout(layoutName string, tileNow bool) error {
	payload, err := json.Marshal(ApplyLayoutPayload{
		LayoutName: layoutName,
		TileNow:    tileNow,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal apply payload: %w", err)
	}

	req := &Request{
		Command: CommandApplyLayout,
		Payload: payload,
	}

	_, err = c.sendRequest(req)
	return err
}

// ApplyLayoutWithOrder sets the daemon's active layout and tiles with a specific window order.
// This is used by workspace load to ensure windows end up in the correct slots.
func (c *Client) ApplyLayoutWithOrder(layoutName string, windowOrder []uint32) error {
	payload, err := json.Marshal(ApplyLayoutPayload{
		LayoutName:  layoutName,
		TileNow:     true,
		WindowOrder: windowOrder,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal apply payload: %w", err)
	}

	req := &Request{
		Command: CommandApplyLayout,
		Payload: payload,
	}

	_, err = c.sendRequest(req)
	return err
}

// SetDefaultLayout updates default_layout in config (optionally tiles immediately).
func (c *Client) SetDefaultLayout(layoutName string, tileNow bool) error {
	payload, err := json.Marshal(SetDefaultLayoutPayload{
		LayoutName: layoutName,
		TileNow:    tileNow,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal set-default payload: %w", err)
	}

	req := &Request{
		Command: CommandSetDefaultLayout,
		Payload: payload,
	}

	_, err = c.sendRequest(req)
	return err
}

// Ping checks if the daemon is responding
func (c *Client) Ping() error {
	_, err := c.GetStatus()
	return err
}
