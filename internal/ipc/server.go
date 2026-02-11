package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/runtimepath"
	"github.com/1broseidon/termtile/internal/tiling"
)

// Server handles IPC requests from clients
type Server struct {
	socketPath   string
	listener     net.Listener
	cfg          *config.Config
	cfgMu        sync.RWMutex
	tiler        *tiling.Tiler
	backend      platform.Backend
	startTime    time.Time
	reloadChan   chan struct{}
	shuttingDown bool
	shutdownMu   sync.Mutex
}

// NewServer creates a new IPC server
func NewServer(cfg *config.Config, tiler *tiling.Tiler, backend platform.Backend, reloadChan chan struct{}) (*Server, error) {
	socketPath, err := runtimepath.SocketPath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve IPC socket path: %w", err)
	}

	// Remove existing socket if present
	os.Remove(socketPath)

	return &Server{
		socketPath: socketPath,
		cfg:        cfg,
		tiler:      tiler,
		backend:    backend,
		startTime:  time.Now(),
		reloadChan: reloadChan,
	}, nil
}

// Start begins listening for IPC connections
func (s *Server) Start() error {
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create IPC socket: %w", err)
	}
	s.listener = listener

	// Set socket permissions
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	log.Printf("IPC server listening on %s", s.socketPath)

	// Accept connections
	go s.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.shutdownMu.Lock()
			if s.shuttingDown {
				s.shutdownMu.Unlock()
				return
			}
			s.shutdownMu.Unlock()
			log.Printf("IPC accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a single IPC connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read the request (expect JSON on a single line)
	data, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		log.Printf("IPC read error: %v", err)
		return
	}

	// Parse request
	req, err := ParseRequest(data)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// Handle command
	resp := s.handleCommand(req)

	// Send response
	respData, err := resp.Marshal()
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	respData = append(respData, '\n')
	if _, err := conn.Write(respData); err != nil {
		log.Printf("Failed to send response: %v", err)
	}
}

// handleCommand processes an IPC command and returns a response
func (s *Server) handleCommand(req *Request) *Response {
	switch req.Command {
	case CommandReload:
		return s.handleReload()
	case CommandGetStatus:
		return s.handleGetStatus()
	case CommandGetMonitors:
		return s.handleGetMonitors()
	case CommandPreviewLayout:
		return s.handlePreviewLayout(req.Payload)
	case CommandListLayouts:
		return s.handleListLayouts()
	case CommandApplyLayout:
		return s.handleApplyLayout(req.Payload)
	case CommandSetDefaultLayout:
		return s.handleSetDefaultLayout(req.Payload)
	case CommandUndo:
		return s.handleUndo()
	default:
		return NewErrorResponse(fmt.Sprintf("Unknown command: %s", req.Command))
	}
}

// handleReload reloads the configuration
func (s *Server) handleReload() *Response {
	log.Println("IPC: Received RELOAD command")

	// Load new config
	newCfg, err := config.Load()
	if err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to reload config: %v", err))
	}

	// Update config atomically
	s.cfgMu.Lock()
	s.cfg = newCfg
	s.cfgMu.Unlock()

	// Notify the main daemon via channel (non-blocking)
	select {
	case s.reloadChan <- struct{}{}:
	default:
	}

	log.Println("IPC: Config reloaded successfully")

	resp, _ := NewOKResponse(nil)
	return resp
}

// handleGetStatus returns current daemon status
func (s *Server) handleGetStatus() *Response {
	// Get active monitor workspace
	display, err := s.backend.ActiveDisplay()
	terminalCount := 0
	if err == nil {
		terminalCount = s.tiler.GetTerminalCount(display.ID)
	}

	status := StatusData{
		ActiveLayout:  s.tiler.GetActiveLayoutName(),
		TerminalCount: terminalCount,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
		DaemonRunning: true,
	}

	resp, _ := NewOKResponse(status)
	return resp
}

// handleGetMonitors returns information about all monitors
func (s *Server) handleGetMonitors() *Response {
	displays, err := s.backend.Displays()
	if err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to get monitors: %v", err))
	}

	monitorInfos := make([]MonitorInfo, len(displays))
	for i, d := range displays {
		monitorInfos[i] = MonitorInfo{
			ID:     d.ID,
			Name:   d.Name,
			X:      d.Bounds.X,
			Y:      d.Bounds.Y,
			Width:  d.Bounds.Width,
			Height: d.Bounds.Height,
		}
	}

	data := MonitorsData{
		Monitors: monitorInfos,
	}

	resp, _ := NewOKResponse(data)
	return resp
}

// handlePreviewLayout temporarily applies a layout for preview
func (s *Server) handlePreviewLayout(payload json.RawMessage) *Response {
	var previewReq PreviewLayoutPayload
	if err := json.Unmarshal(payload, &previewReq); err != nil {
		return NewErrorResponse(fmt.Sprintf("Invalid preview payload: %v", err))
	}

	s.cfgMu.RLock()
	layoutName := previewReq.LayoutName
	if layoutName == "" {
		layoutName = s.cfg.DefaultLayout
	}
	s.cfgMu.RUnlock()

	duration := time.Duration(previewReq.DurationSeconds) * time.Second
	if duration <= 0 {
		duration = 3 * time.Second
	}
	if duration > 60*time.Second {
		duration = 60 * time.Second
	}

	log.Printf("IPC: Preview layout '%s' for %s", layoutName, duration)

	if err := s.tiler.PreviewLayout(layoutName, duration); err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to preview layout: %v", err))
	}

	resp, _ := NewOKResponse(nil)
	return resp
}

func (s *Server) handleListLayouts() *Response {
	s.cfgMu.RLock()
	layoutNames := make([]string, 0, len(s.cfg.Layouts))
	for name := range s.cfg.Layouts {
		layoutNames = append(layoutNames, name)
	}
	defaultLayout := s.cfg.DefaultLayout
	s.cfgMu.RUnlock()

	sort.Strings(layoutNames)

	data := LayoutsData{
		Layouts:       layoutNames,
		DefaultLayout: defaultLayout,
		ActiveLayout:  s.tiler.GetActiveLayoutName(),
	}

	resp, _ := NewOKResponse(data)
	return resp
}

func (s *Server) handleApplyLayout(payload json.RawMessage) *Response {
	var req ApplyLayoutPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewErrorResponse(fmt.Sprintf("Invalid apply payload: %v", err))
	}
	if req.LayoutName == "" {
		return NewErrorResponse("layout_name is required")
	}

	if err := s.tiler.SetActiveLayout(req.LayoutName); err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to set active layout: %v", err))
	}

	if req.TileNow {
		var err error
		if len(req.WindowOrder) > 0 {
			// Use provided window order instead of sorting by position
			err = s.tiler.TileWithOrder(req.WindowOrder)
		} else {
			err = s.tiler.TileCurrentMonitor()
		}
		if err != nil {
			return NewErrorResponse(fmt.Sprintf("Failed to tile with active layout: %v", err))
		}
	}

	resp, _ := NewOKResponse(nil)
	return resp
}

func (s *Server) handleSetDefaultLayout(payload json.RawMessage) *Response {
	var req SetDefaultLayoutPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return NewErrorResponse(fmt.Sprintf("Invalid set default payload: %v", err))
	}
	if req.LayoutName == "" {
		return NewErrorResponse("layout_name is required")
	}

	s.cfgMu.Lock()
	if _, ok := s.cfg.Layouts[req.LayoutName]; !ok {
		s.cfgMu.Unlock()
		return NewErrorResponse(fmt.Sprintf("Unknown layout: %s", req.LayoutName))
	}
	s.cfg.DefaultLayout = req.LayoutName
	err := s.cfg.Save()
	s.cfgMu.Unlock()
	if err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to save config: %v", err))
	}

	_ = s.tiler.SetActiveLayout(req.LayoutName)
	if req.TileNow {
		if err := s.tiler.TileCurrentMonitor(); err != nil {
			return NewErrorResponse(fmt.Sprintf("Failed to tile with default layout: %v", err))
		}
	}

	resp, _ := NewOKResponse(nil)
	return resp
}

func (s *Server) handleUndo() *Response {
	if err := s.tiler.UndoCurrentMonitor(); err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to undo: %v", err))
	}

	resp, _ := NewOKResponse(nil)
	return resp
}

// sendError sends an error response
func (s *Server) sendError(conn net.Conn, errMsg string) {
	resp := NewErrorResponse(errMsg)
	data, _ := resp.Marshal()
	data = append(data, '\n')
	conn.Write(data)
}

// Stop gracefully shuts down the IPC server
func (s *Server) Stop() {
	s.shutdownMu.Lock()
	s.shuttingDown = true
	s.shutdownMu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
}

// GetConfig returns the current config (thread-safe)
func (s *Server) GetConfig() *config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

// UpdateConfig updates the config (thread-safe)
func (s *Server) UpdateConfig(cfg *config.Config) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg = cfg
}
