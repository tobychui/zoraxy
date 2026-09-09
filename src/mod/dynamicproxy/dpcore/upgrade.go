package dpcore

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

/*
	Upgrade.go

	This script handles generic HTTP protocol upgrades in dpcore,
	e.g. Tailscale TS2021 "tailscale-control-protocol". WebSocket upgrades are
	handled by the websocketproxy in a higher layer abstraction and never reach here.

	Added in Zoraxy v3.3.5 by tobychui
*/

// upgradeType returns the Upgrade header value if the Connection header requests an upgrade
func upgradeType(header http.Header) string {
	if !headerContainsToken(header, "Connection", "upgrade") {
		return ""
	}
	return header.Get("Upgrade")
}

// headerContainsToken checks if a comma separated header contains the given token, case insensitive
func headerContainsToken(header http.Header, key string, token string) bool {
	for _, value := range header.Values(key) {
		for _, field := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(field), token) {
				return true
			}
		}
	}
	return false
}

// isPrintableASCII checks a header value is safe to echo back into a status line
func isPrintableASCII(s string) bool {
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return s != ""
}

// restoreUpgradeHeaders re-adds the upgrade headers stripped by the hop-by-hop removal
func restoreUpgradeHeaders(header http.Header, upgradeType string) {
	header.Set("Connection", "Upgrade")
	header.Set("Upgrade", upgradeType)
}

// handleUpgradeResponse hijacks the client connection and tunnels bytes bidirectionally
// between the client and the upstream after a 101 Switching Protocols response
func (p *ReverseProxy) handleUpgradeResponse(rw http.ResponseWriter, reqUpType string, res *http.Response) (int, error) {
	resUpType := upgradeType(res.Header)
	if !isPrintableASCII(resUpType) {
		res.Body.Close()
		return http.StatusBadGateway, errors.New("upstream switched to an invalid protocol")
	}

	if !strings.EqualFold(reqUpType, resUpType) {
		res.Body.Close()
		return http.StatusBadGateway, errors.New("upstream switched to protocol " + resUpType + " when " + reqUpType + " was requested")
	}

	//For a 101 response the transport hands us the raw upstream connection
	backendConn, ok := res.Body.(io.ReadWriteCloser)
	if !ok {
		res.Body.Close()
		return http.StatusBadGateway, errors.New("upstream returned 101 with a non-writable body")
	}
	defer backendConn.Close()

	//HTTP/2 downstream cannot be hijacked, so an upgrade can never complete on it
	clientConn, clientBuf, err := http.NewResponseController(rw).Hijack()
	if err != nil {
		return http.StatusBadGateway, errors.New("connection upgrade requires a hijackable connection: " + err.Error())
	}
	defer clientConn.Close()

	//The client may have pipelined bytes before we hijacked, forward them first
	if buffered := clientBuf.Reader.Buffered(); buffered > 0 {
		if _, err := io.CopyN(backendConn, clientBuf, int64(buffered)); err != nil {
			return http.StatusBadGateway, errors.New("failed to forward buffered client bytes: " + err.Error())
		}
	}

	if _, err := clientBuf.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		return http.StatusBadGateway, err
	}
	if err := res.Header.Write(clientBuf); err != nil {
		return http.StatusBadGateway, err
	}
	if _, err := clientBuf.WriteString("\r\n"); err != nil {
		return http.StatusBadGateway, err
	}
	if err := clientBuf.Flush(); err != nil {
		return http.StatusBadGateway, err
	}

	tunnelUpgradedConnection(clientConn, backendConn)
	return http.StatusSwitchingProtocols, nil
}

// tunnelUpgradedConnection copies bytes in both directions until either side closes
func tunnelUpgradedConnection(clientConn net.Conn, backendConn io.ReadWriteCloser) {
	timeout := upgradeTunnelTimeout

	//The hijacked connection inherits the server's deadlines, clear or extend them
	deadline := time.Time{}
	if upgradeTunnelRefreshOnActivity {
		deadline = time.Now().Add(timeout)
	}
	setConnDeadline(clientConn, deadline)
	setConnDeadline(backendConn, deadline)

	done := make(chan struct{}, 2)
	copyAndSignal := func(dst io.Writer, src io.Reader, dstConn io.ReadWriteCloser, srcConn io.ReadWriteCloser) {
		if upgradeTunnelRefreshOnActivity {
			copyWithDeadlineRefresh(dst, src, dstConn, srcConn, timeout)
		} else {
			io.Copy(dst, src)
		}
		done <- struct{}{}
	}

	go copyAndSignal(backendConn, clientConn, backendConn, clientConn)
	go copyAndSignal(clientConn, backendConn, clientConn, backendConn)

	//Either direction closing tears down the tunnel, the deferred Close calls unblock the other
	<-done
}

// copyWithDeadlineRefresh copies src to dst, pushing both deadlines forward on every read
func copyWithDeadlineRefresh(dst io.Writer, src io.Reader, dstConn io.ReadWriteCloser, srcConn io.ReadWriteCloser, timeout time.Duration) {
	buf := make([]byte, 32*1024)
	for {
		nr, rerr := src.Read(buf)
		if nr > 0 {
			deadline := time.Now().Add(timeout)
			setConnDeadline(srcConn, deadline)
			setConnDeadline(dstConn, deadline)
			if _, werr := dst.Write(buf[:nr]); werr != nil {
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

// setConnDeadline sets the deadline if the connection supports it, a zero time clears it
func setConnDeadline(conn interface{}, deadline time.Time) {
	type deadlineSetter interface {
		SetDeadline(t time.Time) error
	}
	if setter, ok := conn.(deadlineSetter); ok {
		_ = setter.SetDeadline(deadline)
	}
}
