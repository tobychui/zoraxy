package cproxy

import (
	"net/http"
	"time"
)

func New(options ...option) http.Handler {
	var this configuration
	Options.apply(options...)(&this)
	return newHandler(this.Filter, this.ClientConnector, this.ServerConnector, this.Monitor)
}

var Options singleton

type singleton struct{}
type option func(*configuration)

type configuration struct {
	DialTimeout     time.Duration
	Filter          Filter
	DialAddress     string
	Dialer          Dialer
	LogConnections  bool
	ProxyProtocol   bool
	Initializer     initializer
	ClientConnector clientConnector
	ServerConnector serverConnector
	Monitor         monitor
	Logger          logger
}

func (singleton) DialTimeout(value time.Duration) option {
	return func(this *configuration) { this.DialTimeout = value }
}
func (singleton) Filter(value Filter) option {
	return func(this *configuration) { this.Filter = value }
}
func (singleton) ClientConnector(value clientConnector) option {
	return func(this *configuration) { this.ClientConnector = value }
}
func (singleton) DialAddress(value string) option {
	return func(this *configuration) { this.DialAddress = value }
}
func (singleton) Dialer(value Dialer) option {
	return func(this *configuration) { this.Dialer = value }
}
func (singleton) LogConnections(value bool) option {
	return func(this *configuration) { this.LogConnections = value }
}
func (singleton) ProxyProtocol(value bool) option {
	return func(this *configuration) { this.ProxyProtocol = value }
}
func (singleton) Initializer(value initializer) option {
	return func(this *configuration) { this.Initializer = value }
}
func (singleton) ServerConnector(value serverConnector) option {
	return func(this *configuration) { this.ServerConnector = value }
}
func (singleton) Monitor(value monitor) option {
	return func(this *configuration) { this.Monitor = value }
}
func (singleton) Logger(value logger) option {
	return func(this *configuration) { this.Logger = value }
}

func (singleton) apply(options ...option) option {
	return func(this *configuration) {
		for _, item := range Options.defaults(options...) {
			item(this)
		}

		if this.Dialer == nil {
			this.Dialer = newDialer(this)
		}

		this.Dialer = newRoutingDialer(this)

		if this.ProxyProtocol {
			this.Initializer = newProxyProtocolInitializer()
		}

		if this.Initializer == nil {
			this.Initializer = nop{}
		}

		this.Initializer = newLoggingInitializer(this)

		if this.ServerConnector == nil {
			this.ServerConnector = newServerConnector(this.Dialer, this.Initializer)
		}
	}
}
func (singleton) defaults(options ...option) []option {
	return append([]option{
		Options.DialTimeout(time.Second * 10),
		Options.Filter(newFilter()),
		Options.ClientConnector(newClientConnector()),
		Options.Initializer(nop{}),
		Options.Monitor(nop{}),
		Options.Logger(nop{}),
	}, options...)
}

type nop struct{}

func (nop) Measure(int)                    {}
func (nop) Printf(string, ...interface{})  {}
func (nop) Initialize(Socket, Socket) bool { return true }
