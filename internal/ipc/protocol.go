package ipc

import (
	"encoding/json"
	"fmt"
)

// CommandType represents different IPC command types
type CommandType string

const (
	CommandReload           CommandType = "RELOAD"
	CommandGetStatus        CommandType = "GET_STATUS"
	CommandGetMonitors      CommandType = "GET_MONITORS"
	CommandPreviewLayout    CommandType = "PREVIEW_LAYOUT"
	CommandListLayouts      CommandType = "LIST_LAYOUTS"
	CommandApplyLayout      CommandType = "APPLY_LAYOUT"
	CommandSetDefaultLayout CommandType = "SET_DEFAULT_LAYOUT"
	CommandUndo             CommandType = "UNDO"
)

// Request represents an IPC request from client to server
type Request struct {
	Command CommandType     `json:"command"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response represents an IPC response from server to client
type Response struct {
	Status string          `json:"status"` // "OK" or "ERROR"
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// StatusData represents the data returned by GET_STATUS
type StatusData struct {
	ActiveLayout  string `json:"active_layout"`
	TerminalCount int    `json:"terminal_count"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	DaemonRunning bool   `json:"daemon_running"`
}

// MonitorInfo represents information about a single monitor
type MonitorInfo struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// MonitorsData represents the data returned by GET_MONITORS
type MonitorsData struct {
	Monitors []MonitorInfo `json:"monitors"`
}

// PreviewLayoutPayload represents the payload for PREVIEW_LAYOUT command
type PreviewLayoutPayload struct {
	LayoutName      string `json:"layout_name"`
	DurationSeconds int    `json:"duration_seconds"`
}

type LayoutsData struct {
	Layouts       []string `json:"layouts"`
	DefaultLayout string   `json:"default_layout"`
	ActiveLayout  string   `json:"active_layout"`
}

type ApplyLayoutPayload struct {
	LayoutName  string   `json:"layout_name"`
	TileNow     bool     `json:"tile_now,omitempty"`
	WindowOrder []uint32 `json:"window_order,omitempty"` // If set, use this window order instead of sorting
}

type SetDefaultLayoutPayload struct {
	LayoutName string `json:"layout_name"`
	TileNow    bool   `json:"tile_now,omitempty"`
}

// NewOKResponse creates a successful response with optional data
func NewOKResponse(data interface{}) (*Response, error) {
	var dataBytes json.RawMessage
	if data != nil {
		bytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response data: %w", err)
		}
		dataBytes = bytes
	}

	return &Response{
		Status: "OK",
		Data:   dataBytes,
	}, nil
}

// NewErrorResponse creates an error response with a message
func NewErrorResponse(errMsg string) *Response {
	return &Response{
		Status: "ERROR",
		Error:  errMsg,
	}
}

// ParseRequest parses a request from JSON bytes
func ParseRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}
	return &req, nil
}

// Marshal converts a response to JSON bytes
func (r *Response) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
