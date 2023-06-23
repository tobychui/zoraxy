package netutils

import (
	"net"
	"strings"
	"time"
)

type WHOISResult struct {
	DomainName       string    `json:"domainName"`
	RegistryDomainID string    `json:"registryDomainID"`
	Registrar        string    `json:"registrar"`
	UpdatedDate      time.Time `json:"updatedDate"`
	CreationDate     time.Time `json:"creationDate"`
	ExpiryDate       time.Time `json:"expiryDate"`
	RegistrantID     string    `json:"registrantID"`
	RegistrantName   string    `json:"registrantName"`
	RegistrantEmail  string    `json:"registrantEmail"`
	AdminID          string    `json:"adminID"`
	AdminName        string    `json:"adminName"`
	AdminEmail       string    `json:"adminEmail"`
	TechID           string    `json:"techID"`
	TechName         string    `json:"techName"`
	TechEmail        string    `json:"techEmail"`
	NameServers      []string  `json:"nameServers"`
	DNSSEC           string    `json:"dnssec"`
}

func ParseWHOISResponse(response string) (WHOISResult, error) {
	result := WHOISResult{}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Domain Name:") {
			result.DomainName = strings.TrimSpace(strings.TrimPrefix(line, "Domain Name:"))
		} else if strings.HasPrefix(line, "Registry Domain ID:") {
			result.RegistryDomainID = strings.TrimSpace(strings.TrimPrefix(line, "Registry Domain ID:"))
		} else if strings.HasPrefix(line, "Registrar:") {
			result.Registrar = strings.TrimSpace(strings.TrimPrefix(line, "Registrar:"))
		} else if strings.HasPrefix(line, "Updated Date:") {
			dateStr := strings.TrimSpace(strings.TrimPrefix(line, "Updated Date:"))
			updatedDate, err := time.Parse("2006-01-02T15:04:05Z", dateStr)
			if err == nil {
				result.UpdatedDate = updatedDate
			}
		} else if strings.HasPrefix(line, "Creation Date:") {
			dateStr := strings.TrimSpace(strings.TrimPrefix(line, "Creation Date:"))
			creationDate, err := time.Parse("2006-01-02T15:04:05Z", dateStr)
			if err == nil {
				result.CreationDate = creationDate
			}
		} else if strings.HasPrefix(line, "Registry Expiry Date:") {
			dateStr := strings.TrimSpace(strings.TrimPrefix(line, "Registry Expiry Date:"))
			expiryDate, err := time.Parse("2006-01-02T15:04:05Z", dateStr)
			if err == nil {
				result.ExpiryDate = expiryDate
			}
		} else if strings.HasPrefix(line, "Registry Registrant ID:") {
			result.RegistrantID = strings.TrimSpace(strings.TrimPrefix(line, "Registry Registrant ID:"))
		} else if strings.HasPrefix(line, "Registrant Name:") {
			result.RegistrantName = strings.TrimSpace(strings.TrimPrefix(line, "Registrant Name:"))
		} else if strings.HasPrefix(line, "Registrant Email:") {
			result.RegistrantEmail = strings.TrimSpace(strings.TrimPrefix(line, "Registrant Email:"))
		} else if strings.HasPrefix(line, "Registry Admin ID:") {
			result.AdminID = strings.TrimSpace(strings.TrimPrefix(line, "Registry Admin ID:"))
		} else if strings.HasPrefix(line, "Admin Name:") {
			result.AdminName = strings.TrimSpace(strings.TrimPrefix(line, "Admin Name:"))
		} else if strings.HasPrefix(line, "Admin Email:") {
			result.AdminEmail = strings.TrimSpace(strings.TrimPrefix(line, "Admin Email:"))
		} else if strings.HasPrefix(line, "Registry Tech ID:") {
			result.TechID = strings.TrimSpace(strings.TrimPrefix(line, "Registry Tech ID:"))
		} else if strings.HasPrefix(line, "Tech Name:") {
			result.TechName = strings.TrimSpace(strings.TrimPrefix(line, "Tech Name:"))
		} else if strings.HasPrefix(line, "Tech Email:") {
			result.TechEmail = strings.TrimSpace(strings.TrimPrefix(line, "Tech Email:"))
		} else if strings.HasPrefix(line, "Name Server:") {
			ns := strings.TrimSpace(strings.TrimPrefix(line, "Name Server:"))
			result.NameServers = append(result.NameServers, ns)
		} else if strings.HasPrefix(line, "DNSSEC:") {
			result.DNSSEC = strings.TrimSpace(strings.TrimPrefix(line, "DNSSEC:"))
		}
	}

	return result, nil
}

type WhoisIpLookupEntry struct {
	NetRange     string
	CIDR         string
	NetName      string
	NetHandle    string
	Parent       string
	NetType      string
	OriginAS     string
	Organization Organization
	RegDate      time.Time
	Updated      time.Time
	Ref          string
}

type Organization struct {
	OrgName    string
	OrgId      string
	Address    string
	City       string
	StateProv  string
	PostalCode string
	Country    string
	/*
		RegDate    time.Time
		Updated    time.Time
			OrgTechHandle    string
			OrgTechName      string
			OrgTechPhone     string
			OrgTechEmail     string
			OrgAbuseHandle   string
			OrgAbuseName     string
			OrgAbusePhone    string
			OrgAbuseEmail    string
			OrgRoutingHandle string
			OrgRoutingName   string
			OrgRoutingPhone  string
			OrgRoutingEmail  string
	*/
}

func ParseWhoisIpData(data string) (WhoisIpLookupEntry, error) {
	var entry WhoisIpLookupEntry = WhoisIpLookupEntry{}
	var org Organization = Organization{}

	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "NetRange:") {
			entry.NetRange = strings.TrimSpace(strings.TrimPrefix(line, "NetRange:"))
		} else if strings.HasPrefix(line, "CIDR:") {
			entry.CIDR = strings.TrimSpace(strings.TrimPrefix(line, "CIDR:"))
		} else if strings.HasPrefix(line, "NetName:") {
			entry.NetName = strings.TrimSpace(strings.TrimPrefix(line, "NetName:"))
		} else if strings.HasPrefix(line, "NetHandle:") {
			entry.NetHandle = strings.TrimSpace(strings.TrimPrefix(line, "NetHandle:"))
		} else if strings.HasPrefix(line, "Parent:") {
			entry.Parent = strings.TrimSpace(strings.TrimPrefix(line, "Parent:"))
		} else if strings.HasPrefix(line, "NetType:") {
			entry.NetType = strings.TrimSpace(strings.TrimPrefix(line, "NetType:"))
		} else if strings.HasPrefix(line, "OriginAS:") {
			entry.OriginAS = strings.TrimSpace(strings.TrimPrefix(line, "OriginAS:"))
		} else if strings.HasPrefix(line, "Organization:") {
			org.OrgName = strings.TrimSpace(strings.TrimPrefix(line, "Organization:"))
		} else if strings.HasPrefix(line, "OrgId:") {
			org.OrgId = strings.TrimSpace(strings.TrimPrefix(line, "OrgId:"))
		} else if strings.HasPrefix(line, "Address:") {
			org.Address = strings.TrimSpace(strings.TrimPrefix(line, "Address:"))
		} else if strings.HasPrefix(line, "City:") {
			org.City = strings.TrimSpace(strings.TrimPrefix(line, "City:"))
		} else if strings.HasPrefix(line, "StateProv:") {
			org.StateProv = strings.TrimSpace(strings.TrimPrefix(line, "StateProv:"))
		} else if strings.HasPrefix(line, "PostalCode:") {
			org.PostalCode = strings.TrimSpace(strings.TrimPrefix(line, "PostalCode:"))
		} else if strings.HasPrefix(line, "Country:") {
			org.Country = strings.TrimSpace(strings.TrimPrefix(line, "Country:"))
		} else if strings.HasPrefix(line, "RegDate:") {
			entry.RegDate, _ = parseDate(strings.TrimSpace(strings.TrimPrefix(line, "RegDate:")))
		} else if strings.HasPrefix(line, "Updated:") {
			entry.Updated, _ = parseDate(strings.TrimSpace(strings.TrimPrefix(line, "Updated:")))
		} else if strings.HasPrefix(line, "Ref:") {
			entry.Ref = strings.TrimSpace(strings.TrimPrefix(line, "Ref:"))
		}
	}

	entry.Organization = org
	return entry, nil
}

func parseDate(dateStr string) (time.Time, error) {
	dateLayout := "2006-01-02"
	date, err := time.Parse(dateLayout, strings.TrimSpace(dateStr))
	if err != nil {
		return time.Time{}, err
	}
	return date, nil
}

func isDomainName(input string) bool {
	ip := net.ParseIP(input)
	if ip != nil {
		// Check if it's IPv4 or IPv6
		if ip.To4() != nil {
			return false
		} else if ip.To16() != nil {
			return false
		}
	}

	_, err := net.LookupHost(input)
	return err == nil
}
