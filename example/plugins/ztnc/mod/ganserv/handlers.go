package ganserv

import (
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"

	"aroz.org/zoraxy/ztnc/mod/utils"
)

func (m *NetworkManager) HandleGetNodeID(w http.ResponseWriter, r *http.Request) {
	if m.ControllerID == "" {
		//Node id not exists. Check again
		instanceInfo, err := getControllerInfo(m.option.AuthToken, m.option.ApiPort)
		if err != nil {
			utils.SendErrorResponse(w, "unable to access node id information")
			return
		}

		m.ControllerID = instanceInfo.Address
	}

	js, _ := json.Marshal(m.ControllerID)
	utils.SendJSONResponse(w, string(js))
}

func (m *NetworkManager) HandleAddNetwork(w http.ResponseWriter, r *http.Request) {
	networkInfo, err := m.createNetwork()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Network created. Assign it the standard network settings
	err = m.configureNetwork(networkInfo.Nwid, "192.168.192.1", "192.168.192.254", "192.168.192.0/24")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// Return the new network ID
	js, _ := json.Marshal(networkInfo.Nwid)
	utils.SendJSONResponse(w, string(js))
}

func (m *NetworkManager) HandleRemoveNetwork(w http.ResponseWriter, r *http.Request) {
	networkID, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty network id given")
		return
	}

	if !m.networkExists(networkID) {
		utils.SendErrorResponse(w, "network id not exists")
		return
	}

	err = m.deleteNetwork(networkID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
	}

	utils.SendOK(w)
}

func (m *NetworkManager) HandleListNetwork(w http.ResponseWriter, r *http.Request) {
	netid, _ := utils.GetPara(r, "netid")
	if netid != "" {
		targetNetInfo, err := m.getNetworkInfoById(netid)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(targetNetInfo)
		utils.SendJSONResponse(w, string(js))

	} else {
		// Return the list of networks as JSON
		networkIds, err := m.listNetworkIds()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		networkInfos := []*NetworkInfo{}
		for _, id := range networkIds {
			thisNetInfo, err := m.getNetworkInfoById(id)
			if err == nil {
				networkInfos = append(networkInfos, thisNetInfo)
			}
		}

		js, _ := json.Marshal(networkInfos)
		utils.SendJSONResponse(w, string(js))
	}

}

func (m *NetworkManager) HandleNetworkNaming(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "network id not given")
		return
	}

	if !m.networkExists(netid) {
		utils.SendErrorResponse(w, "network not eixsts")
	}

	newName, _ := utils.PostPara(r, "name")
	newDesc, _ := utils.PostPara(r, "desc")
	if newName != "" && newDesc != "" {
		//Strip away html from name and desc
		re := regexp.MustCompile("<[^>]*>")
		newName := re.ReplaceAllString(newName, "")
		newDesc := re.ReplaceAllString(newDesc, "")

		//Set the new network name and desc
		err = m.setNetworkNameAndDescription(netid, newName, newDesc)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		utils.SendOK(w)
	} else {
		//Get current name and description
		name, desc, err := m.getNetworkNameAndDescription(netid)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal([]string{name, desc})
		utils.SendJSONResponse(w, string(js))
	}
}

func (m *NetworkManager) HandleNetworkDetails(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "netid not given")
		return
	}

	targetNetwork, err := m.getNetworkInfoById(netid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(targetNetwork)
	utils.SendJSONResponse(w, string(js))
}

func (m *NetworkManager) HandleSetRanges(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "netid not given")
		return
	}
	cidr, err := utils.PostPara(r, "cidr")
	if err != nil {
		utils.SendErrorResponse(w, "cidr not given")
		return
	}
	ipstart, err := utils.PostPara(r, "ipstart")
	if err != nil {
		utils.SendErrorResponse(w, "ipstart not given")
		return
	}
	ipend, err := utils.PostPara(r, "ipend")
	if err != nil {
		utils.SendErrorResponse(w, "ipend not given")
		return
	}

	//Validate the CIDR is real, the ip range is within the CIDR range
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		utils.SendErrorResponse(w, "invalid cidr string given")
		return
	}

	startIP := net.ParseIP(ipstart)
	endIP := net.ParseIP(ipend)
	if startIP == nil || endIP == nil {
		utils.SendErrorResponse(w, "invalid start or end ip given")
		return
	}

	withinRange := ipnet.Contains(startIP) && ipnet.Contains(endIP)
	if !withinRange {
		utils.SendErrorResponse(w, "given CIDR did not cover all of the start to end ip range")
		return
	}

	err = m.configureNetwork(netid, startIP.String(), endIP.String(), strings.TrimSpace(cidr))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Handle listing of network members. Set details=true for listing all details
func (m *NetworkManager) HandleMemberList(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.GetPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "netid is empty")
		return
	}

	details, _ := utils.GetPara(r, "detail")

	memberIds, err := m.getNetworkMembers(netid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if details == "" {
		//Only show client ids
		js, _ := json.Marshal(memberIds)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Show detail members info
		detailMemberInfo := []*MemberInfo{}
		for _, thisMemberId := range memberIds {
			memInfo, err := m.getNetworkMemberInfo(netid, thisMemberId)
			if err == nil {
				detailMemberInfo = append(detailMemberInfo, memInfo)
			}
		}

		js, _ := json.Marshal(detailMemberInfo)
		utils.SendJSONResponse(w, string(js))
	}
}

// Handle Authorization of members
func (m *NetworkManager) HandleMemberAuthorization(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	memberid, err := utils.PostPara(r, "memid")
	if err != nil {
		utils.SendErrorResponse(w, "memid not set")
		return
	}

	//Check if the target memeber exists
	if !m.memberExistsInNetwork(netid, memberid) {
		utils.SendErrorResponse(w, "member not exists in given network")
		return
	}

	setAuthorized, err := utils.PostPara(r, "auth")
	if err != nil || setAuthorized == "" {
		//Get the member authorization state
		memberInfo, err := m.getNetworkMemberInfo(netid, memberid)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(memberInfo.Authorized)
		utils.SendJSONResponse(w, string(js))
	} else if setAuthorized == "true" {
		m.AuthorizeMember(netid, memberid, true)
	} else if setAuthorized == "false" {
		m.AuthorizeMember(netid, memberid, false)
	} else {
		utils.SendErrorResponse(w, "unknown operation state: "+setAuthorized)
	}
}

// Handle Delete or Add IP for a member in a network
func (m *NetworkManager) HandleMemberIP(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	memberid, err := utils.PostPara(r, "memid")
	if err != nil {
		utils.SendErrorResponse(w, "memid not set")
		return
	}

	opr, err := utils.PostPara(r, "opr")
	if err != nil {
		utils.SendErrorResponse(w, "opr not defined")
		return
	}

	targetip, _ := utils.PostPara(r, "ip")

	memberInfo, err := m.getNetworkMemberInfo(netid, memberid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if opr == "add" {
		if targetip == "" {
			utils.SendErrorResponse(w, "ip not set")
			return
		}

		if !isValidIPAddr(targetip) {
			utils.SendErrorResponse(w, "ip address not valid")
			return
		}

		newIpList := append(memberInfo.IPAssignments, targetip)
		err = m.setAssignedIps(netid, memberid, newIpList)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)

	} else if opr == "del" {
		if targetip == "" {
			utils.SendErrorResponse(w, "ip not set")
			return
		}

		//Delete user ip from the list
		newIpList := []string{}
		for _, thisIp := range memberInfo.IPAssignments {
			if thisIp != targetip {
				newIpList = append(newIpList, thisIp)
			}
		}

		err = m.setAssignedIps(netid, memberid, newIpList)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)
	} else if opr == "get" {
		js, _ := json.Marshal(memberInfo.IPAssignments)
		utils.SendJSONResponse(w, string(js))
	} else {
		utils.SendErrorResponse(w, "unsupported opr type: "+opr)
	}
}

// Handle naming for members
func (m *NetworkManager) HandleMemberNaming(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	memberid, err := utils.PostPara(r, "memid")
	if err != nil {
		utils.SendErrorResponse(w, "memid not set")
		return
	}

	if !m.memberExistsInNetwork(netid, memberid) {
		utils.SendErrorResponse(w, "target member not exists in given network")
		return
	}

	//Read memeber data
	targetMemberData := m.GetMemberMetaData(netid, memberid)

	newname, err := utils.PostPara(r, "name")
	if err != nil {
		//Send over the member data
		js, _ := json.Marshal(targetMemberData)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Write member data
		targetMemberData.Name = newname
		m.WriteMemeberMetaData(netid, memberid, targetMemberData)
		utils.SendOK(w)
	}
}

// Handle delete of a given memver
func (m *NetworkManager) HandleMemberDelete(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	memberid, err := utils.PostPara(r, "memid")
	if err != nil {
		utils.SendErrorResponse(w, "memid not set")
		return
	}

	//Check if that member is authorized.
	memberInfo, err := m.getNetworkMemberInfo(netid, memberid)
	if err != nil {
		utils.SendErrorResponse(w, "member not exists in given GANet")
		return
	}

	if memberInfo.Authorized {
		//Deauthorized this member before deleting
		m.AuthorizeMember(netid, memberid, false)
	}

	//Remove the memeber
	err = m.deleteMember(netid, memberid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Check if a given network id is a network hosted on this zoraxy node
func (m *NetworkManager) IsLocalGAN(networkId string) bool {
	networks, err := m.listNetworkIds()
	if err != nil {
		return false
	}

	for _, network := range networks {
		if network == networkId {
			return true
		}
	}

	return false
}

// Handle server instant joining a given network
func (m *NetworkManager) HandleServerJoinNetwork(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	//Check if the target network is a network hosted on this server
	if !m.IsLocalGAN(netid) {
		utils.SendErrorResponse(w, "given network is not a GAN hosted on this node")
		return
	}

	if m.memberExistsInNetwork(netid, m.ControllerID) {
		utils.SendErrorResponse(w, "controller already inside network")
		return
	}

	//Join the network
	err = m.joinNetwork(netid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Handle server instant leaving a given network
func (m *NetworkManager) HandleServerLeaveNetwork(w http.ResponseWriter, r *http.Request) {
	netid, err := utils.PostPara(r, "netid")
	if err != nil {
		utils.SendErrorResponse(w, "net id not set")
		return
	}

	//Check if the target network is a network hosted on this server
	if !m.IsLocalGAN(netid) {
		utils.SendErrorResponse(w, "given network is not a GAN hosted on this node")
		return
	}

	//Leave the network
	err = m.leaveNetwork(netid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Remove it from target network if it is authorized
	err = m.deleteMember(netid, m.ControllerID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
