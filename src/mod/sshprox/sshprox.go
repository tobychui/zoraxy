package sshprox

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/reverseproxy"
	"imuslab.com/zoraxy/mod/utils"
	"imuslab.com/zoraxy/mod/websocketproxy"
)

/*
	SSH Proxy

	This is a tool to bind gotty into Zoraxy
	so that you can do something similar to
	online ssh terminal
*/

type Manager struct {
	StartingPort int
	Instances    []*Instance
}

type Instance struct {
	UUID         string
	ExecPath     string
	RemoteAddr   string
	RemotePort   int
	AssignedPort int
	conn         *reverseproxy.ReverseProxy //HTTP proxy
	tty          *exec.Cmd                  //SSH connection ported to web interface
	Parent       *Manager
}

func NewSSHProxyManager() *Manager {
	return &Manager{
		StartingPort: 14810,
		Instances:    []*Instance{},
	}
}

// Get the next free port in the list
func (m *Manager) GetNextPort() int {
	nextPort := m.StartingPort
	occupiedPort := make(map[int]bool)
	for _, instance := range m.Instances {
		occupiedPort[instance.AssignedPort] = true
	}
	for {
		if !occupiedPort[nextPort] {
			return nextPort
		}
		nextPort++
	}
}

func (m *Manager) HandleHttpByInstanceId(instanceId string, w http.ResponseWriter, r *http.Request) {
	targetInstance, err := m.GetInstanceById(instanceId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if targetInstance.tty == nil {
		//Server side already closed
		http.Error(w, "Connection already closed", http.StatusInternalServerError)
		return
	}

	r.Header.Set("X-Forwarded-Host", r.Host)
	requestURL := r.URL.String()
	if r.Header["Upgrade"] != nil && strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
		//Handle WebSocket request. Forward the custom Upgrade header and rewrite origin
		r.Header.Set("A-Upgrade", "websocket")
		requestURL = strings.TrimPrefix(requestURL, "/")
		u, _ := url.Parse("ws://127.0.0.1:" + strconv.Itoa(targetInstance.AssignedPort) + "/" + requestURL)
		wspHandler := websocketproxy.NewProxy(u, false)
		wspHandler.ServeHTTP(w, r)
		return
	}

	targetInstance.conn.ProxyHTTP(w, r)
}

func (m *Manager) GetInstanceById(instanceId string) (*Instance, error) {
	for _, instance := range m.Instances {
		if instance.UUID == instanceId {
			return instance, nil
		}
	}
	return nil, fmt.Errorf("instance not found: %s", instanceId)
}
func (m *Manager) NewSSHProxy(binaryRoot string) (*Instance, error) {
	//Check if the binary exists in system/gotty/
	binary := "gotty_" + runtime.GOOS + "_" + runtime.GOARCH

	if runtime.GOOS == "windows" {
		binary = binary + ".exe"
	}

	//Extract it from embedfs if not exists locally
	execPath := filepath.Join(binaryRoot, binary)

	//Create the storage folder structure
	os.MkdirAll(filepath.Dir(execPath), 0775)

	//Create config file if not exists
	if !utils.FileExists(filepath.Join(filepath.Dir(execPath), ".gotty")) {
		configFile, _ := gotty.ReadFile("gotty/.gotty")
		os.WriteFile(filepath.Join(filepath.Dir(execPath), ".gotty"), configFile, 0775)
	}

	//Create web.ssh binary if not exists
	if !utils.FileExists(execPath) {
		//Try to extract it from embedded fs
		executable, err := gotty.ReadFile("gotty/" + binary)
		if err != nil {
			//Binary not found in embedded
			return nil, errors.New("platform not supported")
		}

		//Extract to target location
		err = os.WriteFile(execPath, executable, 0777)
		if err != nil {
			//Binary not found in embedded
			log.Println("Extract web.ssh failed: " + err.Error())
			return nil, errors.New("web.ssh sub-program extract failed")
		}
	}

	//Convert the binary path to realpath
	realpath, err := filepath.Abs(execPath)
	if err != nil {
		return nil, err
	}

	thisInstance := Instance{
		UUID:         uuid.New().String(),
		ExecPath:     realpath,
		AssignedPort: -1,
		Parent:       m,
	}

	m.Instances = append(m.Instances, &thisInstance)

	return &thisInstance, nil
}

// Create a new Connection to target address
func (i *Instance) CreateNewConnection(listenPort int, username string, remoteIpAddr string, remotePort int) error {
	//Create a gotty instance
	connAddr := remoteIpAddr
	if username != "" {
		connAddr = username + "@" + remoteIpAddr
	}
	configPath := filepath.Join(filepath.Dir(i.ExecPath), ".gotty")
	title := username + "@" + remoteIpAddr
	if remotePort != 22 {
		title = title + ":" + strconv.Itoa(remotePort)
	}

	sshCommand := []string{"ssh", "-t", connAddr, "-p", strconv.Itoa(remotePort)}
	cmd := exec.Command(i.ExecPath, "-w", "-p", strconv.Itoa(listenPort), "--once", "--config", configPath, "--title-format", title, "bash", "-c", strings.Join(sshCommand, " "))
	cmd.Dir = filepath.Dir(i.ExecPath)
	cmd.Env = append(os.Environ(), "TERM=xterm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	go func() {
		cmd.Run()
		i.Destroy()
	}()
	i.tty = cmd
	i.AssignedPort = listenPort
	i.RemoteAddr = remoteIpAddr
	i.RemotePort = remotePort

	//Create a new proxy agent for this root
	path, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(listenPort))
	if err != nil {
		return err
	}

	//Create new proxy objects to the proxy
	proxy := reverseproxy.NewReverseProxy(path)

	i.conn = proxy
	return nil
}

func (i *Instance) Destroy() {
	// Remove the instance from the Manager's Instances list
	for idx, inst := range i.Parent.Instances {
		if inst == i {
			// Remove the instance from the slice by swapping it with the last instance and slicing the slice
			i.Parent.Instances[len(i.Parent.Instances)-1], i.Parent.Instances[idx] = i.Parent.Instances[idx], i.Parent.Instances[len(i.Parent.Instances)-1]
			i.Parent.Instances = i.Parent.Instances[:len(i.Parent.Instances)-1]
			break
		}
	}
}
