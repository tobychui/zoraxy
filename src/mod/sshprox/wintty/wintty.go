package wintty

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// Set to true to trace the full lifecycle of wintty server
const Debug = false

// debugLog is a helper function for debug logging that only logs when Debug is true
func debugLog(format string, args ...interface{}) {
	if Debug {
		log.Printf("[wintty-debug] "+format, args...)
	}
}

// logf is a wrapper for log.Printf
func logf(format string, args ...interface{}) {
	log.Printf("[wintty] "+format, args...)
}

//go:embed index.html
var webUI embed.FS

//go:embed assets
var assets embed.FS

// Config holds the configuration for a wintty instance
type Config struct {
	RemoteAddr string // Remote SSH server address
	RemotePort int    // Remote SSH server port
	Username   string // SSH username
}

// Server represents a wintty web SSH server
type Server struct {
	config     Config
	listenPort int
	listener   net.Listener
	server     *http.Server
	running    bool
	stopped    bool
	mu         sync.Mutex
	OnClose    func() // Callback when server closes unexpectedly
	title      string
}

// SSHClient manages the SSH connection and session
type SSHClient struct {
	conn    *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	stderr  io.Reader
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for proxied connections
	},
}

// NewServer creates a new wintty server instance
func NewServer(config Config, listenPort int) *Server {
	title := config.Username + "@" + config.RemoteAddr
	if config.RemotePort != 22 {
		title = title + ":" + strconv.Itoa(config.RemotePort)
	}
	debugLog("NewServer: creating server for %s on port %d", title, listenPort)
	return &Server{
		config:     config,
		listenPort: listenPort,
		title:      title,
	}
}

// Start starts the web server. It binds the port synchronously so any
// port-conflict error is returned immediately to the caller.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	debugLog("Start: attempting to start server for %s on port %d", s.title, s.listenPort)

	if s.running {
		debugLog("Start: server already running, returning error")
		return fmt.Errorf("server already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/assets/", s.handleAssets)

	// Bind the listener synchronously so we can detect port conflicts
	// before returning to the caller.
	addr := "127.0.0.1:" + strconv.Itoa(s.listenPort)
	debugLog("Start: binding listener on %s", addr)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		debugLog("Start: FAILED to bind %s: %v", addr, err)
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	s.listener = ln
	debugLog("Start: listener bound successfully on %s", addr)

	s.server = &http.Server{
		Handler: mux,
	}

	s.running = true
	s.stopped = false

	go func() {
		debugLog("Start: goroutine serving HTTP on %s", addr)
		// Serve blocks until the listener is closed.
		if err := s.server.Serve(ln); err != http.ErrServerClosed {
			logf("server error on port %d: %v", s.listenPort, err)
			debugLog("Start: Serve() returned unexpected error: %v", err)
		} else {
			debugLog("Start: Serve() returned ErrServerClosed (normal shutdown)")
		}

		s.mu.Lock()
		s.running = false
		wasStopped := s.stopped
		s.mu.Unlock()

		debugLog("Start: goroutine exiting, wasStopped=%v, OnClose=%v", wasStopped, s.OnClose != nil)

		// Only fire OnClose when the server was NOT explicitly stopped
		// (i.e. an unexpected shutdown). Explicit Stop() is called by
		// Destroy(), so we must not re-enter Destroy() here.
		if !wasStopped && s.OnClose != nil {
			debugLog("Start: firing OnClose callback")
			s.OnClose()
		}
	}()

	logf("server started on %s for %s", addr, s.title)
	return nil
}

// Stop explicitly stops the web server. It sets the stopped flag so
// the OnClose callback is NOT fired (the caller is responsible for
// cleanup when calling Stop directly).
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	debugLog("Stop: called for port %d, running=%v, server=%v", s.listenPort, s.running, s.server != nil)

	if !s.running || s.server == nil {
		debugLog("Stop: nothing to stop (not running or nil server)")
		return nil
	}

	s.stopped = true
	logf("stopping server on port %d", s.listenPort)
	err := s.server.Close()
	debugLog("Stop: server.Close() returned: %v", err)
	return err
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// handleIndex serves the main HTML page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	content, err := webUI.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Failed to load page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// handleAssets serves static assets (CSS, JS) from the embedded filesystem
func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	// Read the file from embedded assets
	content, err := assets.ReadFile(r.URL.Path[1:]) // Remove leading slash
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set appropriate content type based on file extension
	contentType := "application/octet-stream"
	if len(r.URL.Path) > 4 {
		switch {
		case r.URL.Path[len(r.URL.Path)-4:] == ".css":
			contentType = "text/css; charset=utf-8"
		case r.URL.Path[len(r.URL.Path)-3:] == ".js":
			contentType = "application/javascript; charset=utf-8"
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}

// handleConfig returns the server configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"title":      s.title,
		"remoteAddr": s.config.RemoteAddr,
		"remotePort": s.config.RemotePort,
		"username":   s.config.Username,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleWebSocket handles WebSocket connections for the terminal
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	debugLog("handleWebSocket: new connection from %s", r.RemoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logf("WebSocket upgrade error: %v", err)
		debugLog("handleWebSocket: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Request password from client via WebSocket channel
	debugLog("handleWebSocket: sending auth_required to client")
	authReq := map[string]string{"type": "auth_required"}
	authReqData, _ := json.Marshal(authReq)
	if err := conn.WriteMessage(websocket.TextMessage, authReqData); err != nil {
		logf("WebSocket write error (auth_required): %v", err)
		return
	}

	// Wait for password from client with a timeout
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		debugLog("handleWebSocket: failed to read auth message: %v", err)
		s.sendError(conn, "Authentication timeout or read error")
		return
	}
	conn.SetReadDeadline(time.Time{}) // Clear deadline

	var authMsg struct {
		Type     string `json:"type"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(message, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Password == "" {
		debugLog("handleWebSocket: invalid auth message")
		s.sendError(conn, "Invalid authentication message")
		return
	}
	password := authMsg.Password
	debugLog("handleWebSocket: received password via WebSocket channel")

	// Connect to SSH server
	debugLog("handleWebSocket: connecting SSH to %s@%s:%d", s.config.Username, s.config.RemoteAddr, s.config.RemotePort)
	sshClient, err := s.connectSSH(password)
	if err != nil {
		debugLog("handleWebSocket: SSH connection failed: %v", err)
		s.sendError(conn, fmt.Sprintf("SSH connection failed: %v", err))
		return
	}
	defer sshClient.Close()
	debugLog("handleWebSocket: SSH connected successfully")

	// Notify client that authentication succeeded
	authOk := map[string]string{"type": "auth_success"}
	authOkData, _ := json.Marshal(authOk)
	if err := conn.WriteMessage(websocket.TextMessage, authOkData); err != nil {
		logf("WebSocket write error (auth_success): %v", err)
		return
	}

	// Create channels for coordinating shutdown
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() {
		once.Do(func() {
			close(done)
		})
	}

	// Start goroutine to read from SSH and write to WebSocket
	go func() {
		defer closeDone()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := sshClient.stdout.Read(buf)
				if err != nil {
					if err != io.EOF {
						logf("SSH read error: %v", err)
					}
					return
				}
				if n > 0 {
					// Ensure valid UTF-8 output
					data := sanitizeUTF8(buf[:n])
					if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
						logf("WebSocket write error: %v", err)
						return
					}
				}
			}
		}
	}()

	// Read from WebSocket and write to SSH
	for {
		select {
		case <-done:
			return
		default:
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logf("WebSocket read error: %v", err)
				}
				closeDone()
				return
			}

			if messageType == websocket.TextMessage {
				// Check for resize message
				if len(message) > 0 && message[0] == '{' {
					var msg struct {
						Type string `json:"type"`
						Cols int    `json:"cols"`
						Rows int    `json:"rows"`
					}
					if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
						if err := sshClient.session.WindowChange(msg.Rows, msg.Cols); err != nil {
							logf("Window resize error: %v", err)
						}
						continue
					}
				}

				// Write input to SSH
				if _, err := sshClient.stdin.Write(message); err != nil {
					logf("SSH write error: %v", err)
					closeDone()
					return
				}
			}
		}
	}
}

// connectSSH establishes an SSH connection with the configured server
func (s *Server) connectSSH(password string) (*SSHClient, error) {
	debugLog("connectSSH: dialling %s:%d as user %s", s.config.RemoteAddr, s.config.RemotePort, s.config.Username)
	config := &ssh.ClientConfig{
		User: s.config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = password
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", s.config.RemoteAddr, s.config.RemotePort)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		debugLog("connectSSH: dial failed: %v", err)
		return nil, fmt.Errorf("failed to dial: %v", err)
	}
	debugLog("connectSSH: TCP+SSH handshake complete")

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	// Set up pseudo terminal
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("failed to request pty: %v", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("failed to get stdin: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("failed to get stdout: %v", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		debugLog("connectSSH: failed to start shell: %v", err)
		return nil, fmt.Errorf("failed to start shell: %v", err)
	}

	debugLog("connectSSH: shell started, session ready")
	return &SSHClient{
		conn:    client,
		session: session,
		stdin:   stdin,
		stdout:  stdout,
	}, nil
}

// sendError sends an error message to the WebSocket client
func (s *Server) sendError(conn *websocket.Conn, message string) {
	errMsg := map[string]string{"error": message}
	data, _ := json.Marshal(errMsg)
	conn.WriteMessage(websocket.TextMessage, data)
}

// Close closes the SSH client connection
func (c *SSHClient) Close() {
	if c.session != nil {
		c.session.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// sanitizeUTF8 ensures the byte slice contains valid UTF-8
func sanitizeUTF8(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	// Replace invalid sequences with replacement character
	result := make([]byte, 0, len(data))
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			result = append(result, '?')
			data = data[1:]
		} else {
			result = append(result, data[:size]...)
			data = data[size:]
		}
	}
	return result
}
