package cproxy

type defaultServerConnector struct {
	dialer      Dialer
	initializer initializer
}

func newServerConnector(dialer Dialer, initializer initializer) *defaultServerConnector {
	return &defaultServerConnector{dialer: dialer, initializer: initializer}
}

func (this *defaultServerConnector) Connect(client Socket, serverAddress string) proxy {
	server := this.dialer.Dial(serverAddress)
	if server == nil {
		return nil
	}

	if !this.initializer.Initialize(client, server) {
		_ = server.Close()
		return nil
	}

	return newProxy(client, server)
}
