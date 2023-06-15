package netutils

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const (
	protocolICMP = 1
)

// liveTraceRoute return realtime tracing information to live response handler
func liveTraceRoute(dst string, maxHops int, liveRespHandler func(string)) error {
	timeout := time.Second * 3
	// resolve the host name to an IP address
	ipAddr, err := net.ResolveIPAddr("ip4", dst)
	if err != nil {
		return fmt.Errorf("failed to resolve IP address for %s: %v", dst, err)
	}
	// create a socket to listen for incoming ICMP packets
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("failed to create ICMP listener: %v", err)
	}
	defer conn.Close()
	id := os.Getpid() & 0xffff
	seq := 0
loop_ttl:
	for ttl := 1; ttl <= maxHops; ttl++ {
		// set the TTL on the socket
		if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
			return fmt.Errorf("failed to set TTL: %v", err)
		}
		seq++
		// create an ICMP message
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   id,
				Seq:  seq,
				Data: []byte("zoraxy_trace"),
			},
		}
		// serialize the ICMP message
		msgBytes, err := msg.Marshal(nil)
		if err != nil {
			return fmt.Errorf("failed to serialize ICMP message: %v", err)
		}
		// send the ICMP message
		start := time.Now()
		if _, err := conn.WriteTo(msgBytes, ipAddr); err != nil {
			//log.Printf("%d: %v", ttl, err)
			liveRespHandler(fmt.Sprintf("%d: %v", ttl, err))
			continue loop_ttl
		}
		// listen for the reply
		replyBytes := make([]byte, 1500)
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return fmt.Errorf("failed to set read deadline: %v", err)
		}
		for i := 0; i < 3; i++ {
			n, peer, err := conn.ReadFrom(replyBytes)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					//fmt.Printf("%d: *\n", ttl)
					liveRespHandler(fmt.Sprintf("%d: *\n", ttl))
					continue loop_ttl
				} else {
					liveRespHandler(fmt.Sprintf("%d: Failed to parse ICMP message: %v", ttl, err))
				}
				continue
			}
			// parse the ICMP message
			replyMsg, err := icmp.ParseMessage(protocolICMP, replyBytes[:n])
			if err != nil {
				liveRespHandler(fmt.Sprintf("%d: Failed to parse ICMP message: %v", ttl, err))
				continue
			}
			// check if the reply is an echo reply
			if replyMsg.Type == ipv4.ICMPTypeEchoReply {
				echoReply, ok := msg.Body.(*icmp.Echo)
				if !ok || echoReply.ID != id || echoReply.Seq != seq {
					continue
				}
				liveRespHandler(fmt.Sprintf("%d: %v %v\n", ttl, peer, time.Since(start)))
				break loop_ttl
			}
			if replyMsg.Type == ipv4.ICMPTypeTimeExceeded {
				echoReply, ok := msg.Body.(*icmp.Echo)
				if !ok || echoReply.ID != id || echoReply.Seq != seq {
					continue
				}
				var raddr = peer.String()
				names, _ := net.LookupAddr(raddr)
				if len(names) > 0 {
					raddr = names[0] + " (" + raddr + ")"
				} else {
					raddr = raddr + " (" + raddr + ")"
				}
				liveRespHandler(fmt.Sprintf("%d: %v %v\n", ttl, raddr, time.Since(start)))
				continue loop_ttl
			}
		}

	}
	return nil
}

// Standard traceroute, return results after complete
func traceroute(dst string, maxHops int) ([]string, error) {
	results := []string{}
	timeout := time.Second * 3
	// resolve the host name to an IP address
	ipAddr, err := net.ResolveIPAddr("ip4", dst)
	if err != nil {
		return results, fmt.Errorf("failed to resolve IP address for %s: %v", dst, err)
	}
	// create a socket to listen for incoming ICMP packets
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return results, fmt.Errorf("failed to create ICMP listener: %v", err)
	}
	defer conn.Close()
	id := os.Getpid() & 0xffff
	seq := 0
loop_ttl:
	for ttl := 1; ttl <= maxHops; ttl++ {
		// set the TTL on the socket
		if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
			return results, fmt.Errorf("failed to set TTL: %v", err)
		}
		seq++
		// create an ICMP message
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   id,
				Seq:  seq,
				Data: []byte("zoraxy_trace"),
			},
		}
		// serialize the ICMP message
		msgBytes, err := msg.Marshal(nil)
		if err != nil {
			return results, fmt.Errorf("failed to serialize ICMP message: %v", err)
		}
		// send the ICMP message
		start := time.Now()
		if _, err := conn.WriteTo(msgBytes, ipAddr); err != nil {
			//log.Printf("%d: %v", ttl, err)
			results = append(results, fmt.Sprintf("%d: %v", ttl, err))
			continue loop_ttl
		}
		// listen for the reply
		replyBytes := make([]byte, 1500)
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return results, fmt.Errorf("failed to set read deadline: %v", err)
		}
		for i := 0; i < 3; i++ {
			n, peer, err := conn.ReadFrom(replyBytes)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					//fmt.Printf("%d: *\n", ttl)
					results = append(results, fmt.Sprintf("%d: *", ttl))
					continue loop_ttl
				} else {
					results = append(results, fmt.Sprintf("%d: Failed to parse ICMP message: %v", ttl, err))
				}
				continue
			}
			// parse the ICMP message
			replyMsg, err := icmp.ParseMessage(protocolICMP, replyBytes[:n])
			if err != nil {
				results = append(results, fmt.Sprintf("%d: Failed to parse ICMP message: %v", ttl, err))
				continue
			}
			// check if the reply is an echo reply
			if replyMsg.Type == ipv4.ICMPTypeEchoReply {
				echoReply, ok := msg.Body.(*icmp.Echo)
				if !ok || echoReply.ID != id || echoReply.Seq != seq {
					continue
				}
				results = append(results, fmt.Sprintf("%d: %v %v", ttl, peer, time.Since(start)))
				break loop_ttl
			}
			if replyMsg.Type == ipv4.ICMPTypeTimeExceeded {
				echoReply, ok := msg.Body.(*icmp.Echo)
				if !ok || echoReply.ID != id || echoReply.Seq != seq {
					continue
				}
				var raddr = peer.String()
				names, _ := net.LookupAddr(raddr)
				if len(names) > 0 {
					raddr = names[0] + " (" + raddr + ")"
				} else {
					raddr = raddr + " (" + raddr + ")"
				}
				results = append(results, fmt.Sprintf("%d: %v %v", ttl, raddr, time.Since(start)))
				continue loop_ttl
			}
		}

	}
	return results, nil
}
