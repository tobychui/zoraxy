package wakeonlan

import (
	"errors"
	"net"
	"time"
)

/*
	Wake On Lan
	Author: tobychui

	This module send wake on LAN signal to a given MAC address
	and do nothing else
*/

type magicPacket [102]byte

func WakeTarget(macAddr string) error {
	packet := magicPacket{}
	mac, err := net.ParseMAC(macAddr)
	if err != nil {
		return err
	}

	if len(mac) != 6 {
		return errors.New("invalid MAC address")
	}

	//Initialize the packet with all F
	copy(packet[0:], []byte{255, 255, 255, 255, 255, 255})
	offset := 6

	for i := 0; i < 16; i++ {
		copy(packet[offset:], mac)
		offset += 6
	}

	//Most devices listen to either port 7 or 9, send to both of them
	err = sendPacket("255.255.255.255:7", packet)
	if err != nil {
		return err
	}

	time.Sleep(30 * time.Millisecond)

	err = sendPacket("255.255.255.255:9", packet)
	if err != nil {
		return err
	}
	return nil
}

func sendPacket(addr string, packet magicPacket) error {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(packet[:])
	return err
}

func IsValidMacAddress(macaddr string) bool {
	_, err := net.ParseMAC(macaddr)
	return err == nil
}
