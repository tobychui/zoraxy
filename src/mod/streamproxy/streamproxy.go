package streamproxy

import (
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/database"
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
	Database             *database.Database
	DefaultTimeout       int
	AccessControlHandler func(net.Conn) bool
}

type Manager struct {
	//Config and stores
	Options *Options
	Configs []*ProxyRelayConfig

	//Realtime Statistics
	Connections int //currently connected connect counts

}

func NewStreamProxy(options *Options) *Manager {
	options.Database.NewTable("tcprox")

	//Load relay configs from db
	previousRules := []*ProxyRelayConfig{}
	if options.Database.KeyExists("tcprox", "rules") {
		options.Database.Read("tcprox", "rules", &previousRules)
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
	}

	thisManager.Configs = previousRules

	return &thisManager
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

func (m *Manager) SaveConfigToDatabase() {
	m.Options.Database.Write("tcprox", "rules", m.Configs)
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
	c.tcpStopChan = tcpStopChan

	udpStopChan := make(chan bool)
	c.udpStopChan = udpStopChan

	//Start the proxy service
	if c.UseUDP {
		go func() {
			if !c.UseTCP {
				//By default running state shows TCP proxy. If TCP is not in use, UDP is shown instead
				c.Running = true
			}
			err := c.ForwardUDP(c.ListeningAddress, c.ProxyTargetAddr, udpStopChan)
			if err != nil {
				if !c.UseTCP {
					c.Running = false
				}
				log.Println("[TCP] Error starting stream proxy " + c.Name + "(" + c.UUID + "): " + err.Error())
			}
		}()
	}

	if c.UseTCP {
		go func() {
			//Default to transport mode
			c.Running = true
			err := c.Port2host(c.ListeningAddress, c.ProxyTargetAddr, tcpStopChan)
			if err != nil {
				c.Running = false
				log.Println("[TCP] Error starting stream proxy " + c.Name + "(" + c.UUID + "): " + err.Error())
			}
		}()
	}

	//Successfully spawned off the proxy routine

	return nil
}

// Stop a running proxy if running
func (c *ProxyRelayConfig) IsRunning() bool {
	return c.tcpStopChan != nil || c.udpStopChan != nil
}

// Stop a running proxy if running
func (c *ProxyRelayConfig) Stop() {
	log.Println("[PROXY] Stopping Stream Proxy " + c.Name)

	if c.udpStopChan != nil {
		c.udpStopChan <- true
		c.udpStopChan = nil
	}

	if c.tcpStopChan != nil {
		c.tcpStopChan <- true
		c.tcpStopChan = nil
	}

	log.Println("[PROXY] Stopped Stream Proxy " + c.Name)
	c.Running = false
}
