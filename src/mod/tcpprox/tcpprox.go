package tcpprox

import (
	"errors"
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

const (
	ProxyMode_Listen    = 0
	ProxyMode_Transport = 1
	ProxyMode_Starter   = 2
	ProxyMode_UDP       = 3
)

type ProxyRelayOptions struct {
	Name    string
	PortA   string
	PortB   string
	Timeout int
	Mode    int
}

type ProxyRelayConfig struct {
	UUID                        string       //A UUIDv4 representing this config
	Name                        string       //Name of the config
	Running                     bool         //If the service is running
	PortA                       string       //Ports A (config depends on mode)
	PortB                       string       //Ports B (config depends on mode)
	Mode                        int          //Operation Mode
	Timeout                     int          //Timeout for connection in sec
	stopChan                    chan bool    //Stop channel to stop the listener
	aTobAccumulatedByteTransfer atomic.Int64 //Accumulated byte transfer from A to B
	bToaAccumulatedByteTransfer atomic.Int64 //Accumulated byte transfer from B to A

	parent *Manager `json:"-"`
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
	Connections  int      //currently connected connect counts
	UDPClientMap sync.Map //map storing the UDP client-server connections
}

func NewTCProxy(options *Options) *Manager {
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
		Running:                     false,
		PortA:                       config.PortA,
		PortB:                       config.PortB,
		Mode:                        config.Mode,
		Timeout:                     config.Timeout,
		stopChan:                    nil,
		aTobAccumulatedByteTransfer: aAcc,
		bToaAccumulatedByteTransfer: bAcc,

		parent: m,
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
func (m *Manager) EditConfig(configUUID string, newName string, newPortA string, newPortB string, newMode int, newTimeout int) error {
	// Find the config with the specified UUID
	foundConfig, err := m.GetConfigByUUID(configUUID)
	if err != nil {
		return err
	}

	// Validate and update the fields
	if newName != "" {
		foundConfig.Name = newName
	}
	if newPortA != "" {
		foundConfig.PortA = newPortA
	}
	if newPortB != "" {
		foundConfig.PortB = newPortB
	}
	if newMode != -1 {
		if newMode > 2 || newMode < 0 {
			return errors.New("invalid mode given")
		}
		foundConfig.Mode = newMode
	}
	if newTimeout != -1 {
		if newTimeout < 0 {
			return errors.New("invalid timeout value given")
		}
		foundConfig.Timeout = newTimeout
	}

	/*
		err = foundConfig.ValidateConfigs()
		if err != nil {
			return err
		}
	*/

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
