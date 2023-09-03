package ganserv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

/*
	zerotier.go

	This hold the functions that required to communicate with
	a zerotier instance

	See more on
	https://docs.zerotier.com/self-hosting/network-controllers/

*/

type NodeInfo struct {
	Address string `json:"address"`
	Clock   int64  `json:"clock"`
	Config  struct {
		Settings struct {
			AllowTCPFallbackRelay bool   `json:"allowTcpFallbackRelay"`
			PortMappingEnabled    bool   `json:"portMappingEnabled"`
			PrimaryPort           int    `json:"primaryPort"`
			SoftwareUpdate        string `json:"softwareUpdate"`
			SoftwareUpdateChannel string `json:"softwareUpdateChannel"`
		} `json:"settings"`
	} `json:"config"`
	Online               bool   `json:"online"`
	PlanetWorldID        int    `json:"planetWorldId"`
	PlanetWorldTimestamp int64  `json:"planetWorldTimestamp"`
	PublicIdentity       string `json:"publicIdentity"`
	TCPFallbackActive    bool   `json:"tcpFallbackActive"`
	Version              string `json:"version"`
	VersionBuild         int    `json:"versionBuild"`
	VersionMajor         int    `json:"versionMajor"`
	VersionMinor         int    `json:"versionMinor"`
	VersionRev           int    `json:"versionRev"`
}

type ErrResp struct {
	Message string `json:"message"`
}

type NetworkInfo struct {
	AuthTokens            []interface{} `json:"authTokens"`
	AuthorizationEndpoint string        `json:"authorizationEndpoint"`
	Capabilities          []interface{} `json:"capabilities"`
	ClientID              string        `json:"clientId"`
	CreationTime          int64         `json:"creationTime"`
	DNS                   []interface{} `json:"dns"`
	EnableBroadcast       bool          `json:"enableBroadcast"`
	ID                    string        `json:"id"`
	IPAssignmentPools     []interface{} `json:"ipAssignmentPools"`
	Mtu                   int           `json:"mtu"`
	MulticastLimit        int           `json:"multicastLimit"`
	Name                  string        `json:"name"`
	Nwid                  string        `json:"nwid"`
	Objtype               string        `json:"objtype"`
	Private               bool          `json:"private"`
	RemoteTraceLevel      int           `json:"remoteTraceLevel"`
	RemoteTraceTarget     interface{}   `json:"remoteTraceTarget"`
	Revision              int           `json:"revision"`
	Routes                []interface{} `json:"routes"`
	Rules                 []struct {
		Not  bool   `json:"not"`
		Or   bool   `json:"or"`
		Type string `json:"type"`
	} `json:"rules"`
	RulesSource  string        `json:"rulesSource"`
	SsoEnabled   bool          `json:"ssoEnabled"`
	Tags         []interface{} `json:"tags"`
	V4AssignMode struct {
		Zt bool `json:"zt"`
	} `json:"v4AssignMode"`
	V6AssignMode struct {
		SixPlane bool `json:"6plane"`
		Rfc4193  bool `json:"rfc4193"`
		Zt       bool `json:"zt"`
	} `json:"v6AssignMode"`
}

type MemberInfo struct {
	ActiveBridge                 bool          `json:"activeBridge"`
	Address                      string        `json:"address"`
	AuthenticationExpiryTime     int           `json:"authenticationExpiryTime"`
	Authorized                   bool          `json:"authorized"`
	Capabilities                 []interface{} `json:"capabilities"`
	CreationTime                 int64         `json:"creationTime"`
	ID                           string        `json:"id"`
	Identity                     string        `json:"identity"`
	IPAssignments                []string      `json:"ipAssignments"`
	LastAuthorizedCredential     interface{}   `json:"lastAuthorizedCredential"`
	LastAuthorizedCredentialType string        `json:"lastAuthorizedCredentialType"`
	LastAuthorizedTime           int           `json:"lastAuthorizedTime"`
	LastDeauthorizedTime         int           `json:"lastDeauthorizedTime"`
	NoAutoAssignIps              bool          `json:"noAutoAssignIps"`
	Nwid                         string        `json:"nwid"`
	Objtype                      string        `json:"objtype"`
	RemoteTraceLevel             int           `json:"remoteTraceLevel"`
	RemoteTraceTarget            interface{}   `json:"remoteTraceTarget"`
	Revision                     int           `json:"revision"`
	SsoExempt                    bool          `json:"ssoExempt"`
	Tags                         []interface{} `json:"tags"`
	VMajor                       int           `json:"vMajor"`
	VMinor                       int           `json:"vMinor"`
	VProto                       int           `json:"vProto"`
	VRev                         int           `json:"vRev"`
}

// Get the zerotier node info from local service
func getControllerInfo(token string, apiPort int) (*NodeInfo, error) {
	url := "http://localhost:" + strconv.Itoa(apiPort) + "/status"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-ZT1-AUTH", token)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	//Read from zerotier service instance

	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	//Parse the payload into struct
	thisInstanceInfo := NodeInfo{}
	err = json.Unmarshal(payload, &thisInstanceInfo)
	if err != nil {
		return nil, err
	}

	return &thisInstanceInfo, nil
}

/*
	Network Functions
*/
//Create a zerotier network
func (m *NetworkManager) createNetwork() (*NetworkInfo, error) {
	url := fmt.Sprintf("http://localhost:"+strconv.Itoa(m.apiPort)+"/controller/network/%s______", m.ControllerID)

	data := []byte(`{}`)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-ZT1-AUTH", m.authToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	networkInfo := NetworkInfo{}
	err = json.Unmarshal(payload, &networkInfo)
	if err != nil {
		return nil, err
	}

	return &networkInfo, nil
}

// List network details
func (m *NetworkManager) getNetworkInfoById(networkId string) (*NetworkInfo, error) {
	req, err := http.NewRequest("GET", os.ExpandEnv("http://localhost:"+strconv.Itoa(m.apiPort)+"/controller/network/"+networkId+"/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Zt1-Auth", m.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("network error. Status code: " + strconv.Itoa(resp.StatusCode))
	}

	thisNetworkInfo := NetworkInfo{}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(payload, &thisNetworkInfo)
	if err != nil {
		return nil, err
	}

	return &thisNetworkInfo, nil
}

func (m *NetworkManager) setNetworkInfoByID(networkId string, newNetworkInfo *NetworkInfo) error {
	payloadBytes, err := json.Marshal(newNetworkInfo)
	if err != nil {
		return err
	}
	payloadBuffer := bytes.NewBuffer(payloadBytes)

	// Create the HTTP request
	url := "http://localhost:" + strconv.Itoa(m.apiPort) + "/controller/network/" + networkId + "/"
	req, err := http.NewRequest("POST", url, payloadBuffer)
	if err != nil {
		return err
	}
	req.Header.Set("X-Zt1-Auth", m.authToken)
	req.Header.Set("Content-Type", "application/json")

	// Send the HTTP request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Print the response status code
	if resp.StatusCode != 200 {
		return errors.New("network error. status code: " + strconv.Itoa(resp.StatusCode))
	}

	return nil
}

// List network IDs
func (m *NetworkManager) listNetworkIds() ([]string, error) {
	req, err := http.NewRequest("GET", "http://localhost:"+strconv.Itoa(m.apiPort)+"/controller/network/", nil)
	if err != nil {
		return []string{}, err
	}
	req.Header.Set("X-Zt1-Auth", m.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return []string{}, errors.New("network error")
	}

	networkIds := []string{}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}, err
	}

	err = json.Unmarshal(payload, &networkIds)
	if err != nil {
		return []string{}, err
	}

	return networkIds, nil
}

// wrapper for checking if a network id exists
func (m *NetworkManager) networkExists(networkId string) bool {
	networkIds, err := m.listNetworkIds()
	if err != nil {
		return false
	}

	for _, thisid := range networkIds {
		if thisid == networkId {
			return true
		}
	}

	return false
}

// delete a network
func (m *NetworkManager) deleteNetwork(networkID string) error {
	url := "http://localhost:" + strconv.Itoa(m.apiPort) + "/controller/network/" + networkID + "/"
	client := &http.Client{}

	// Create a new DELETE request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	// Add the required authorization header
	req.Header.Set("X-Zt1-Auth", m.authToken)

	// Send the request and get the response
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// Close the response body when we're done
	defer resp.Body.Close()
	s, err := io.ReadAll(resp.Body)
	fmt.Println(string(s), err, resp.StatusCode)

	// Print the response status code
	if resp.StatusCode != 200 {
		return errors.New("network error. status code: " + strconv.Itoa(resp.StatusCode))
	}

	return nil
}

// Configure network
// Example: configureNetwork(netid, "192.168.192.1", "192.168.192.254", "192.168.192.0/24")
func (m *NetworkManager) configureNetwork(networkID string, ipRangeStart string, ipRangeEnd string, routeTarget string) error {
	url := "http://localhost:" + strconv.Itoa(m.apiPort) + "/controller/network/" + networkID + "/"
	data := map[string]interface{}{
		"ipAssignmentPools": []map[string]string{
			{
				"ipRangeStart": ipRangeStart,
				"ipRangeEnd":   ipRangeEnd,
			},
		},
		"routes": []map[string]interface{}{
			{
				"target": routeTarget,
				"via":    nil,
			},
		},
		"v4AssignMode": "zt",
		"private":      true,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZT1-AUTH", m.authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	// Print the response status code
	if resp.StatusCode != 200 {
		return errors.New("network error. status code: " + strconv.Itoa(resp.StatusCode))
	}

	return nil
}

func (m *NetworkManager) setAssignedIps(networkID string, memid string, newIps []string) error {
	url := "http://localhost:" + strconv.Itoa(m.apiPort) + "/controller/network/" + networkID + "/member/" + memid
	data := map[string]interface{}{
		"ipAssignments": newIps,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZT1-AUTH", m.authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	// Print the response status code
	if resp.StatusCode != 200 {
		return errors.New("network error. status code: " + strconv.Itoa(resp.StatusCode))
	}

	return nil
}

func (m *NetworkManager) setNetworkNameAndDescription(netid string, name string, desc string) error {
	// Convert string to rune slice
	r := []rune(name)

	// Loop over runes and remove non-ASCII characters
	for i, v := range r {
		if v > 127 {
			r[i] = ' '
		}
	}

	// Convert back to string and trim whitespace
	name = strings.TrimSpace(string(r))

	url := "http://localhost:" + strconv.Itoa(m.apiPort) + "/controller/network/" + netid + "/"
	data := map[string]interface{}{
		"name": name,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZT1-AUTH", m.authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	// Print the response status code
	if resp.StatusCode != 200 {
		return errors.New("network error. status code: " + strconv.Itoa(resp.StatusCode))
	}

	meta := m.GetNetworkMetaData(netid)
	if meta != nil {
		meta.Desc = desc
		m.WriteNetworkMetaData(netid, meta)
	}

	return nil
}

func (m *NetworkManager) getNetworkNameAndDescription(netid string) (string, string, error) {
	//Get name from network info
	netinfo, err := m.getNetworkInfoById(netid)
	if err != nil {
		return "", "", err
	}

	name := netinfo.Name

	//Get description from meta
	desc := ""
	networkMeta := m.GetNetworkMetaData(netid)
	if networkMeta != nil {
		desc = networkMeta.Desc
	}

	return name, desc, nil
}

/*
	Member functions
*/

func (m *NetworkManager) getNetworkMembers(networkId string) ([]string, error) {
	url := fmt.Sprintf("http://localhost:%d/controller/network/%s/member", m.apiPort, networkId)
	reqBody := bytes.NewBuffer([]byte{})
	req, err := http.NewRequest("GET", url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-ZT1-AUTH", m.authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to get network members")
	}

	memberList := map[string]int{}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(payload, &memberList)
	if err != nil {
		return nil, err
	}

	members := make([]string, 0, len(memberList))
	for k := range memberList {
		members = append(members, k)
	}

	return members, nil
}

func (m *NetworkManager) memberExistsInNetwork(netid string, memid string) bool {
	//Get a list of member
	memberids, err := m.getNetworkMembers(netid)
	if err != nil {
		return false
	}
	for _, thisMemberId := range memberids {
		if thisMemberId == memid {
			return true
		}
	}

	return false
}

// Get a network memeber info by netid and memberid
func (m *NetworkManager) getNetworkMemberInfo(netid string, memberid string) (*MemberInfo, error) {
	req, err := http.NewRequest("GET", "http://localhost:"+strconv.Itoa(m.apiPort)+"/controller/network/"+netid+"/member/"+memberid, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Zt1-Auth", m.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	thisMemeberInfo := &MemberInfo{}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(payload, &thisMemeberInfo)
	if err != nil {
		return nil, err
	}

	return thisMemeberInfo, nil
}

// Set the authorization state of a member
func (m *NetworkManager) AuthorizeMember(netid string, memberid string, setAuthorized bool) error {
	url := fmt.Sprintf("http://localhost:%d/controller/network/%s/member/%s", m.apiPort, netid, memberid)
	payload := []byte(`{"authorized": true}`)
	if !setAuthorized {
		payload = []byte(`{"authorized": false}`)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-ZT1-AUTH", m.authToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("network error. Status code: " + strconv.Itoa(resp.StatusCode))
	}

	return nil
}

// Delete a member from the network
func (m *NetworkManager) deleteMember(netid string, memid string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://localhost:%d/controller/network/%s/member/%s", m.apiPort, netid, memid), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Zt1-Auth", os.ExpandEnv(m.authToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("network error. Status code: %v", resp.StatusCode)
	}

	return nil
}
