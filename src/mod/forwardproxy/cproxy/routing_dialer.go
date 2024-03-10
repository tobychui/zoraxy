package cproxy

type routingDialer struct {
	inner         Dialer
	targetAddress string
}

func newRoutingDialer(config *configuration) Dialer {
	if len(config.DialAddress) == 0 {
		return config.Dialer
	}

	return &routingDialer{inner: config.Dialer, targetAddress: config.DialAddress}
}

func (this *routingDialer) Dial(string) Socket {
	return this.inner.Dial(this.targetAddress)
}
