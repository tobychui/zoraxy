package mdns

import (
	"context"
	"log"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
	"imuslab.com/zoraxy/mod/utils"
)

type MDNSHost struct {
	MDNS          *zeroconf.Server
	Host          *NetworkHost
	IfaceOverride *net.Interface
}

type NetworkHost struct {
	HostName     string
	Port         int
	IPv4         []net.IP
	Domain       string
	Model        string
	UUID         string
	Vendor       string
	BuildVersion string
	MacAddr      []string
	Online       bool
}

// Create a new MDNS discoverer, set MacOverride to empty string for using the first NIC discovered
func NewMDNS(config NetworkHost, MacOverride string) (*MDNSHost, error) {
	//Get host MAC Address
	macAddress, err := getMacAddr()
	if err != nil {
		return nil, err
	}

	macAddressBoardcast := ""
	if err == nil {
		macAddressBoardcast = strings.Join(macAddress, ",")
	} else {
		log.Printf("[mDNS] Unable to get MAC Address: %s\n", err.Error())
	}

	//Register the mds services
	server, err := zeroconf.Register(config.HostName, "_http._tcp", "local.", config.Port, []string{"version_build=" + config.BuildVersion, "vendor=" + config.Vendor, "model=" + config.Model, "uuid=" + config.UUID, "domain=" + config.Domain, "mac_addr=" + macAddressBoardcast}, nil)
	if err != nil {
		log.Printf("[mDNS] Error when registering zeroconf broadcast message %s\n", err.Error())
		return &MDNSHost{}, err
	}

	//Discover the iface to override if exists
	var overrideIface *net.Interface = nil
	if MacOverride != "" {
		ifaceIp := ""
		ifaces, err := net.Interfaces()
		if err != nil {
			log.Printf("[mDNS] Unable to override iface MAC: %s. Resuming with default iface\n", err.Error())
		}

		foundMatching := false
		for _, iface := range ifaces {
			thisIfaceMac := iface.HardwareAddr.String()
			thisIfaceMac = strings.ReplaceAll(thisIfaceMac, ":", "-")
			MacOverride = strings.ReplaceAll(MacOverride, ":", "-")
			if strings.EqualFold(thisIfaceMac, strings.TrimSpace(MacOverride)) {
				//This is the correct iface to use
				overrideIface = &iface
				addrs, err := iface.Addrs()

				if err == nil && len(addrs) > 0 {
					ifaceIp = addrs[0].String()
				}

				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}

					if ip.To4() != nil {
						//This NIC have Ipv4 addr
						ifaceIp = ip.String()
					}
				}
				foundMatching = true
				break
			}
		}

		if !foundMatching {
			log.Printf("[mDNS] Unable to find the target iface with MAC address: %s. Resuming with default iface\n", MacOverride)
		} else {
			log.Printf("[mDNS] Entering force MAC address mode, listening on: %s (IP address: %s)\n", MacOverride, ifaceIp)
		}
	}

	return &MDNSHost{
		MDNS:          server,
		Host:          &config,
		IfaceOverride: overrideIface,
	}, nil
}

func (m *MDNSHost) Close() {
	if m != nil {
		m.MDNS.Shutdown()
	}

}

// Scan with given timeout and domain filter. Use m.Host.Domain for scanning similar typed devices
func (m *MDNSHost) Scan(timeout int, domainFilter string) []*NetworkHost {
	// Discover all services on the network (e.g. _workstation._tcp)

	var zcoption zeroconf.ClientOption = nil
	if m.IfaceOverride != nil {
		zcoption = zeroconf.SelectIfaces([]net.Interface{*m.IfaceOverride})
	}

	resolver, err := zeroconf.NewResolver(zcoption)
	if err != nil {
		log.Fatalln("Failed to initialize resolver:", err.Error())
	}

	entries := make(chan *zeroconf.ServiceEntry)
	//Create go routine  to wait for the resolver

	discoveredHost := []*NetworkHost{}

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			if domainFilter == "" {
				//This is a ArOZ Online Host
				//Split the required information out of the text element
				TEXT := entry.Text
				properties := map[string]string{}
				for _, v := range TEXT {
					kv := strings.Split(v, "=")
					if len(kv) == 2 {
						properties[kv[0]] = kv[1]
					}
				}

				var macAddrs []string
				val, ok := properties["mac_addr"]
				if !ok || val == "" {
					//No MacAddr found. Target node version too old
					macAddrs = []string{}
				} else {
					macAddrs = strings.Split(properties["mac_addr"], ",")
				}

				//log.Println(properties)
				discoveredHost = append(discoveredHost, &NetworkHost{
					HostName:     entry.HostName,
					Port:         entry.Port,
					IPv4:         entry.AddrIPv4,
					Domain:       properties["domain"],
					Model:        properties["model"],
					UUID:         properties["uuid"],
					Vendor:       properties["vendor"],
					BuildVersion: properties["version_build"],
					MacAddr:      macAddrs,
					Online:       true,
				})

			} else {
				if utils.StringInArray(entry.Text, "domain="+domainFilter) {
					//This is generic scan request
					//Split the required information out of the text element
					TEXT := entry.Text
					properties := map[string]string{}
					for _, v := range TEXT {
						kv := strings.Split(v, "=")
						if len(kv) == 2 {
							properties[kv[0]] = kv[1]
						}
					}

					var macAddrs []string
					val, ok := properties["mac_addr"]
					if !ok || val == "" {
						//No MacAddr found. Target node version too old
						macAddrs = []string{}
					} else {
						macAddrs = strings.Split(properties["mac_addr"], ",")
					}

					//log.Println(properties)
					discoveredHost = append(discoveredHost, &NetworkHost{
						HostName:     entry.HostName,
						Port:         entry.Port,
						IPv4:         entry.AddrIPv4,
						Domain:       properties["domain"],
						Model:        properties["model"],
						UUID:         properties["uuid"],
						Vendor:       properties["vendor"],
						BuildVersion: properties["version_build"],
						MacAddr:      macAddrs,
						Online:       true,
					})

				}
			}

		}
	}(entries)

	//Resolve each of the mDNS and pipe it back to the log functions
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
	defer cancel()
	err = resolver.Browse(ctx, "_http._tcp", "local.", entries)
	if err != nil {
		log.Fatalln("Failed to browse:", err.Error())
	}

	//Update the master scan record
	<-ctx.Done()
	return discoveredHost
}

// Get all mac address of all interfaces
func getMacAddr() ([]string, error) {
	ifas, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as, nil
}
