package cproxy

type loggingInitializer struct {
	logger logger
	inner  initializer
}

func newLoggingInitializer(config *configuration) initializer {
	if !config.LogConnections {
		return config.Initializer
	}

	return &loggingInitializer{inner: config.Initializer, logger: config.Logger}
}

func (this *loggingInitializer) Initialize(client, server Socket) bool {
	result := this.inner.Initialize(client, server)

	if !result {
		this.logger.Printf("Connection failed [%s] -> [%s]", client.RemoteAddr(), server.RemoteAddr())
	}

	return result
}
