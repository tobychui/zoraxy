package streamproxy

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Stream Proxy

	Forward port from one port to another
	Also accept active connection and passive
	connection
*/

// ProxyProtocolVersion enum type
type ProxyProtocolVersion int

const (
	ProxyProtocolDisabled ProxyProtocolVersion = 0
	ProxyProtocolV1       ProxyProtocolVersion = 1
	ProxyProtocolV2       ProxyProtocolVersion = 2
)

type ProxyRelayOptions struct {
	Name                 string
	ListeningAddr        string
	ProxyAddr            string
	Timeout              int
	UseTCP               bool
	UseUDP               bool
	ProxyProtocolVersion ProxyProtocolVersion
	EnableLogging        bool
}

// ProxyRuleUpdateConfig is used to update the proxy rule config
type ProxyRuleUpdateConfig struct {
	InstanceUUID         string //The target instance UUID to update
	NewName              string //New name for the instance, leave empty for no change
	NewListeningAddr     string //New listening address, leave empty for no change
	NewProxyAddr         string //New proxy target address, leave empty for no change
	UseTCP               bool   //Enable TCP proxy, default to false
	UseUDP               bool   //Enable UDP proxy, default to false
	ProxyProtocolVersion int    //Enable Proxy Protocol v1/v2, default to disabled
	EnableLogging        bool   //Enable Logging TCP/UDP Message, default to true
	NewTimeout           int    //New timeout for the connection, leave -1 for no change
}

type ProxyRelayInstance struct {
	/* Runtime Config */
	UUID                 string               //A UUIDv4 representing this config
	Name                 string               //Name of the config
	Running              bool                 //Status, read only
	AutoStart            bool                 //If the service suppose to started automatically
	ListeningAddress     string               //Listening Address, usually 127.0.0.1:port
	ProxyTargetAddr      string               //Proxy target address
	UseTCP               bool                 //Enable TCP proxy
	UseUDP               bool                 //Enable UDP proxy
	ProxyProtocolVersion ProxyProtocolVersion //Proxy Protocol v1/v2
	EnableLogging        bool                 //Enable logging for ProxyInstance
	Timeout              int                  //Timeout for connection in sec

	/* Internal */
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
	Configs []*ProxyRelayInstance

	//Realtime Statistics
	Connections int //currently connected connect counts

}

// NewStreamProxy creates a new stream proxy manager with the given options
func NewStreamProxy(options *Options) (*Manager, error) {
	if !utils.FileExists(options.ConfigStore) {
		err := os.MkdirAll(options.ConfigStore, 0775)
		if err != nil {
			return nil, err
		}
	}

	//Load relay configs from db
	previousRules := []*ProxyRelayInstance{}
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
		thisRelayConfig := &ProxyRelayInstance{}
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

// NewConfig creates a new proxy relay config with the given options
func (m *Manager) NewConfig(config *ProxyRelayOptions) string {
	//Generate two zero value for atomic int64
	aAcc := atomic.Int64{}
	bAcc := atomic.Int64{}
	aAcc.Store(0)
	bAcc.Store(0)
	//Generate a new config from options
	configUUID := uuid.New().String()
	thisConfig := ProxyRelayInstance{
		UUID:                        configUUID,
		Name:                        config.Name,
		ListeningAddress:            config.ListeningAddr,
		ProxyTargetAddr:             config.ProxyAddr,
		UseTCP:                      config.UseTCP,
		UseUDP:                      config.UseUDP,
		ProxyProtocolVersion:        config.ProxyProtocolVersion,
		EnableLogging:               config.EnableLogging,
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

func (m *Manager) GetConfigByUUID(configUUID string) (*ProxyRelayInstance, error) {
	// Find and return the config with the specified UUID
	for _, config := range m.Configs {
		if config.UUID == configUUID {
			return config, nil
		}
	}
	return nil, errors.New("config not found")
}

// ConvertIntToProxyProtocolVersion converts an int to ProxyProtocolVersion type
func convertIntToProxyProtocolVersion(v int) ProxyProtocolVersion {
	switch v {
	case 1:
		return ProxyProtocolV1
	case 2:
		return ProxyProtocolV2
	default:
		return ProxyProtocolDisabled
	}
}

// convertProxyProtocolVersionToInt converts ProxyProtocolVersion type back to int
func convertProxyProtocolVersionToInt(v ProxyProtocolVersion) int {
	switch v {
	case ProxyProtocolV1:
		return 1
	case ProxyProtocolV2:
		return 2
	default:
		return 0
	}
}

// Edit the config based on config UUID, leave empty for unchange fields
func (m *Manager) EditConfig(newConfig *ProxyRuleUpdateConfig) error {
	// Find the config with the specified UUID
	foundConfig, err := m.GetConfigByUUID(newConfig.InstanceUUID)
	if err != nil {
		return err
	}

	// Validate and update the fields
	if newConfig.NewName != "" {
		foundConfig.Name = newConfig.NewName
	}
	if newConfig.NewListeningAddr != "" {
		foundConfig.ListeningAddress = newConfig.NewListeningAddr
	}
	if newConfig.NewProxyAddr != "" {
		foundConfig.ProxyTargetAddr = newConfig.NewProxyAddr
	}

	foundConfig.UseTCP = newConfig.UseTCP
	foundConfig.UseUDP = newConfig.UseUDP
	foundConfig.ProxyProtocolVersion = convertIntToProxyProtocolVersion(newConfig.ProxyProtocolVersion)
	foundConfig.EnableLogging = newConfig.EnableLogging

	if newConfig.NewTimeout != -1 {
		if newConfig.NewTimeout < 0 {
			return errors.New("invalid timeout value given")
		}
		foundConfig.Timeout = newConfig.NewTimeout
	}

	m.SaveConfigToDatabase()

	//Check if config is running. If yes, restart it
	if foundConfig.IsRunning() {
		foundConfig.Restart()
	}
	return nil
}

// Remove the config from file by UUID
func (m *Manager) RemoveConfig(configUUID string) error {
	err := os.Remove(filepath.Join(m.Options.ConfigStore, configUUID+".config"))
	if err != nil {
		return err
	}
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
