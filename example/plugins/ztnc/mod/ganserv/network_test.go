package ganserv_test

import (
	"fmt"
	"net"
	"strconv"
	"testing"

	"aroz.org/zoraxy/ztnc/mod/ganserv"
)

func TestGetRandomFreeIP(t *testing.T) {
	n := ganserv.Network{
		CIDR: "172.16.0.0/12",
		Nodes: []*ganserv.Node{
			{
				Name:      "nodeC1",
				ManagedIP: net.ParseIP("172.16.1.142"),
			},
			{
				Name:      "nodeC2",
				ManagedIP: net.ParseIP("172.16.5.174"),
			},
		},
	}

	// Call the function for 10 times
	for i := 0; i < 10; i++ {
		freeIP, err := n.GetRandomFreeIP()
		fmt.Println("["+strconv.Itoa(i)+"] Free IP address assigned: ", freeIP)

		// Assert that no error occurred
		if err != nil {
			t.Errorf("Unexpected error: %s", err.Error())
		}

		// Assert that the returned IP is a valid IPv4 address
		if freeIP.To4() == nil {
			t.Errorf("Invalid IP address format: %s", freeIP.String())
		}

		// Assert that the returned IP is not already used by a node
		for _, node := range n.Nodes {
			if freeIP.Equal(node.ManagedIP) {
				t.Errorf("Returned IP is already in use: %s", freeIP.String())
			}
		}

		n.Nodes = append(n.Nodes, &ganserv.Node{
			Name:      "NodeT" + strconv.Itoa(i),
			ManagedIP: freeIP,
		})
	}

}
