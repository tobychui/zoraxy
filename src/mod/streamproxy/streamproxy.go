package streamproxy

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	TCP Proxy

	Forward port from one port to another
	Also accept active connection and passive
	connection
*/

type ProxyRelayOptions struct {
	Name          string
	ListeningAddr string
	ProxyAddr     string
	Timeout       int
	UseTCP        bool
	UseUDP        bool
}

type ProxyRelayConfig struct {
	UUID                        string       //A UUIDv4 representing this config
	Name                        string       //Name of the config
	Running                     bool         //Status, read only
	AutoStart                   bool         //If the service suppose to started automatically
	ListeningAddress            string       //Listening Address, usually 127.0.0.1:port
	ProxyTargetAddr             string       //Proxy target address
	UseTCP                      bool         //Enable TCP proxy
	UseUDP                      bool         //Enable UDP proxy
	Timeout                     int          //Timeout for connection in sec
	tcpStopChan                 chan bool    //Stop channel for TCP listener
	udpStopChan                 chan bool    //Stop channel for UDP listener
	aTobAccumulatedByteTransfer atomic.Int64 //Accumulated byte transfer from A to B
	bToaAccumulatedByteTransfer atomic.Int64 //Accumulated byte transfer from B to A
	udpClientMap                sync.Map     //map storing the UDP client-server connections
	parent                      *Manager     `json:"-"`
}

type Options struct {
	DefaultTimeout       int
	AccessControlHandler func(net.Conn) bool
	ConfigStore          string         //Folder to store the config files, will be created if not exists
	Logger               *logger.Logger //Logger for the stream proxy
}

type Manager struct {
	//Config and stores
	Options *Options
	Configs []*ProxyRelayConfig

	//Realtime Statistics
	Connections int //currently connected connect counts

}

func NewStreamProxy(options *Options) (*Manager, error) {
	if !utils.FileExists(options.ConfigStore) {
		err := os.MkdirAll(options.ConfigStore, 0775)
		if err != nil {
			return nil, err
		}
	}

	//Load relay configs from db
	previousRules := []*ProxyRelayConfig{}
	streamProxyConfigFiles, err := filepath.Glob(options.ConfigStore + "/*.config")
	if err != nil {
		return nil, err
	}

	for _, configFile := range streamProxyConfigFiles {
		//Read file into bytes
		configBytes, err := os.ReadFile(configFile)
		if err != nil {
			options.Logger.PrintAndLog("stream-prox", "Read stream proxy config failed", err)
			continue
		}
		thisRelayConfig := &ProxyRelayConfig{}
		err = json.Unmarshal(configBytes, thisRelayConfig)
		if err != nil {
			options.Logger.PrintAndLog("stream-prox", "Unmarshal stream proxy config failed", err)
			continue
		}

		//Append the config to the list
		previousRules = append(previousRules, thisRelayConfig)
	}

	//Check if the AccessControlHandler is empty. If yes, set it to always allow access
	if options.AccessControlHandler == nil {
		options.AccessControlHandler = func(conn net.Conn) bool {
			//Always allow access
			return true
		}
	}

	//Create a new proxy manager for TCP
	thisManager := Manager{
		Options:     options,
		Connections: 0,
	}

	//Inject manager into the rules
	for _, rule := range previousRules {
		rule.parent = &thisManager
		if rule.Running {
			//This was previously running. Start it again
			thisManager.logf("Resuming stream proxy rule "+rule.Name, nil)
			rule.Start()
		}
	}

	thisManager.Configs = previousRules

	return &thisManager, nil
}

// Wrapper function to log error
func (m *Manager) logf(message string, originalError error) {
	if m.Options.Logger == nil {
		//Print to fmt
		if originalError != nil {
			message += ": " + originalError.Error()
		}
		println(message)
		return
	}
	m.Options.Logger.PrintAndLog("stream-prox", message, originalError)
}

func (m *Manager) NewConfig(config *ProxyRelayOptions) string {
	//Generate two zero value for atomic int64
	aAcc := atomic.Int64{}
	bAcc := atomic.Int64{}
	aAcc.Store(0)
	bAcc.Store(0)
	//Generate a new config from options
	configUUID := uuid.New().String()
	thisConfig := ProxyRelayConfig{
		UUID:                        configUUID,
		Name:                        config.Name,
		ListeningAddress:            config.ListeningAddr,
		ProxyTargetAddr:             config.ProxyAddr,
		UseTCP:                      config.UseTCP,
		UseUDP:                      config.UseUDP,
		Timeout:                     config.Timeout,
		tcpStopChan:                 nil,
		udpStopChan:                 nil,
		aTobAccumulatedByteTransfer: aAcc,
		bToaAccumulatedByteTransfer: bAcc,
		udpClientMap:                sync.Map{},
		parent:                      m,
	}
	m.Configs = append(m.Configs, &thisConfig)
	m.SaveConfigToDatabase()
	return configUUID
}

func (m *Manager) GetConfigByUUID(configUUID string) (*ProxyRelayConfig, error) {
	// Find and return the config with the specified UUID
	for _, config := range m.Configs {
		if config.UUID == configUUID {
			return config, nil
		}
	}
	return nil, errors.New("config not found")
}

// Edit the config based on config UUID, leave empty for unchange fields
func (m *Manager) EditConfig(configUUID string, newName string, newListeningAddr string, newProxyAddr string, useTCP bool, useUDP bool, newTimeout int) error {
	// Find the config with the specified UUID
	foundConfig, err := m.GetConfigByUUID(configUUID)
	if err != nil {
		return err
	}

	// Validate and update the fields
	if newName != "" {
		foundConfig.Name = newName
	}
	if newListeningAddr != "" {
		foundConfig.ListeningAddress = newListeningAddr
	}
	if newProxyAddr != "" {
		foundConfig.ProxyTargetAddr = newProxyAddr
	}

	foundConfig.UseTCP = useTCP
	foundConfig.UseUDP = useUDP

	if newTimeout != -1 {
		if newTimeout < 0 {
			return errors.New("invalid timeout value given")
		}
		foundConfig.Timeout = newTimeout
	}

	m.SaveConfigToDatabase()

	//Check if config is running. If yes, restart it
	if foundConfig.IsRunning() {
		foundConfig.Restart()
	}

	return nil
}

func (m *Manager) RemoveConfig(configUUID string) error {
	// Find and remove the config with the specified UUID
	for i, config := range m.Configs {
		if config.UUID == configUUID {
			m.Configs = append(m.Configs[:i], m.Configs[i+1:]...)
			m.SaveConfigToDatabase()
			return nil
		}
	}
	return errors.New("config not found")
}

// Save all configs to ConfigStore folder
func (m *Manager) SaveConfigToDatabase() {
	for _, config := range m.Configs {
		configBytes, err := json.Marshal(config)
		if err != nil {
			m.logf("Failed to marshal stream proxy config", err)
			continue
		}
		err = os.WriteFile(m.Options.ConfigStore+"/"+config.UUID+".config", configBytes, 0775)
		if err != nil {
			m.logf("Failed to save stream proxy config", err)
		}
	}
}

/*
	Config Functions
*/

// Start a proxy if stopped
func (c *ProxyRelayConfig) Start() error {
	if c.IsRunning() {
		c.Running = true
		return errors.New("proxy already running")
	}

	// Create a stopChan to control the loop
	tcpStopChan := make(chan bool)
	udpStopChan := make(chan bool)

	//Start the proxy service
	if c.UseUDP {
		c.udpStopChan = udpStopChan
		go func() {
			err := c.ForwardUDP(c.ListeningAddress, c.ProxyTargetAddr, udpStopChan)
			if err != nil {
				if !c.UseTCP {
					c.Running = false
					c.udpStopChan = nil
					c.parent.SaveConfigToDatabase()
				}
				c.parent.logf("[proto:udp] Error starting stream proxy "+c.Name+"("+c.UUID+")", err)
			}
		}()
	}

	if c.UseTCP {
		c.tcpStopChan = tcpStopChan
		go func() {
			//Default to transport mode
			err := c.Port2host(c.ListeningAddress, c.ProxyTargetAddr, tcpStopChan)
			if err != nil {
				c.Running = false
				c.tcpStopChan = nil
				c.parent.SaveConfigToDatabase()
				c.parent.logf("[proto:tcp] Error starting stream proxy "+c.Name+"("+c.UUID+")", err)
			}
		}()
	}

	//Successfully spawned off the proxy routine
	c.Running = true
	c.parent.SaveConfigToDatabase()
	return nil
}

// Return if a proxy config is running
func (c *ProxyRelayConfig) IsRunning() bool {
	return c.tcpStopChan != nil || c.udpStopChan != nil
}

// Restart a proxy config
func (c *ProxyRelayConfig) Restart() {
	if c.IsRunning() {
		c.Stop()
	}
	time.Sleep(3000 * time.Millisecond)
	c.Start()
}

// Stop a running proxy if running
func (c *ProxyRelayConfig) Stop() {
	c.parent.logf("Stopping Stream Proxy "+c.Name, nil)

	if c.udpStopChan != nil {
		c.parent.logf("Stopping UDP for "+c.Name, nil)
		c.udpStopChan <- true
		c.udpStopChan = nil
	}

	if c.tcpStopChan != nil {
		c.parent.logf("Stopping TCP for "+c.Name, nil)
		c.tcpStopChan <- true
		c.tcpStopChan = nil
	}

	c.parent.logf("Stopped Stream Proxy "+c.Name, nil)
	c.Running = false

	//Update the running status
	c.parent.SaveConfigToDatabase()
}
