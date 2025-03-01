package ganserv

import (
	"log"
	"net"

	"aroz.org/zoraxy/ztnc/mod/database"
)

/*
	Global Area Network
	Server side implementation

	This module do a few things to help manage
	the system GANs

	- Provide DHCP assign to client
	- Provide a list of connected nodes in the same VLAN
	- Provide proxy of packet if the target VLAN is online but not reachable

	Also provide HTTP Handler functions for management
	- Create Network
	- Update Network Properties (Name / Desc)
	- Delete Network

	- Authorize Node
	- Deauthorize Node
	- Set / Get Network Prefered Subnet Mask
	- Handle Node ping
*/

type Node struct {
	Auth          bool   //If the node is authorized in this network
	ClientID      string //The client ID
	MAC           string //The tap MAC this client is using
	Name          string //Name of the client in this network
	Description   string //Description text
	ManagedIP     net.IP //The IP address assigned by this network
	LastSeen      int64  //Last time it is seen from this host
	ClientVersion string //Client application version
	PublicIP      net.IP //Public IP address as seen from this host
}

type Network struct {
	UID         string  //UUID of the network, must be a 16 char random ASCII string
	Name        string  //Name of the network, ASCII only
	Description string  //Description of the network
	CIDR        string  //The subnet masked use by this network
	Nodes       []*Node //The nodes currently attached in this network
}

type NetworkManagerOptions struct {
	Database  *database.Database
	AuthToken string
	ApiPort   int
}

type NetworkMetaData struct {
	Desc string
}

type MemberMetaData struct {
	Name string
}

type NetworkManager struct {
	authToken        string
	apiPort          int
	ControllerID     string
	option           *NetworkManagerOptions
	networksMetadata map[string]NetworkMetaData
}

// Create a new GAN manager
func NewNetworkManager(option *NetworkManagerOptions) *NetworkManager {
	option.Database.NewTable("ganserv")

	//Load network metadata
	networkMeta := map[string]NetworkMetaData{}
	if option.Database.KeyExists("ganserv", "networkmeta") {
		option.Database.Read("ganserv", "networkmeta", &networkMeta)
	}

	//Start the zerotier instance if not exists

	//Get controller info
	instanceInfo, err := getControllerInfo(option.AuthToken, option.ApiPort)
	if err != nil {
		log.Println("ZeroTier connection failed: ", err.Error())
		return &NetworkManager{
			authToken:        option.AuthToken,
			apiPort:          option.ApiPort,
			ControllerID:     "",
			option:           option,
			networksMetadata: networkMeta,
		}
	}

	return &NetworkManager{
		authToken:        option.AuthToken,
		apiPort:          option.ApiPort,
		ControllerID:     instanceInfo.Address,
		option:           option,
		networksMetadata: networkMeta,
	}
}

func (m *NetworkManager) GetNetworkMetaData(netid string) *NetworkMetaData {
	md, ok := m.networksMetadata[netid]
	if !ok {
		return &NetworkMetaData{}
	}

	return &md
}

func (m *NetworkManager) WriteNetworkMetaData(netid string, meta *NetworkMetaData) {
	m.networksMetadata[netid] = *meta
	m.option.Database.Write("ganserv", "networkmeta", m.networksMetadata)
}

func (m *NetworkManager) GetMemberMetaData(netid string, memid string) *MemberMetaData {
	thisMemberData := MemberMetaData{}
	m.option.Database.Read("ganserv", "memberdata_"+netid+"_"+memid, &thisMemberData)
	return &thisMemberData
}

func (m *NetworkManager) WriteMemeberMetaData(netid string, memid string, meta *MemberMetaData) {
	m.option.Database.Write("ganserv", "memberdata_"+netid+"_"+memid, meta)
}
