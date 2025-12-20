# Zoraxy Modules

This directory contains all the modular components that make up the Zoraxy reverse proxy server. Each module is designed to be independent and focused on a specific functionality, allowing for better maintainability and extensibility.

## Core Modules

### reverseproxy
**Purpose**: Core reverse proxy functionality
- Implements HTTP/HTTPS reverse proxy with support for tunneling
- Handles request forwarding to backend servers
- Provides basic proxy timeout and transport configuration

### dynamicproxy
**Purpose**: Advanced dynamic reverse proxy with comprehensive features
- Manages multiple proxy endpoints with different configurations
- Supports load balancing, sticky sessions, and upstream health monitoring
- Implements access control, authentication, and custom headers
- Provides exploit detection and mitigation
- Handles virtual directories and path rewriting
- Includes rate limiting and traffic management

### webserv
**Purpose**: Static web server functionality
- Serves static files and web content
- Includes file manager capabilities
- Provides middleware support for web requests

## Security & Access Control

### access
**Purpose**: Access control and filtering system
- Implements blacklist and whitelist functionality
- Manages access rules for different endpoints
- Provides loopback protection

### auth
**Purpose**: Authentication and authorization system
- Handles user authentication and session management
- Supports SSO (Single Sign-On) integration
- Manages API keys and authentication middleware
- Provides OAuth2 and basic authentication

### tlscert
**Purpose**: TLS certificate management
- Generates self-signed certificates
- Manages certificate storage and retrieval
- Provides certificate helper utilities

### acme
**Purpose**: ACME protocol implementation for SSL certificates
- Automates SSL certificate issuance and renewal
- Supports multiple Certificate Authorities (Let's Encrypt, etc.)
- Includes DNS challenge support for wildcard certificates

## Networking & Communication

### streamproxy
**Purpose**: TCP/UDP stream proxy
- Proxies TCP and UDP connections
- Supports various streaming protocols
- Provides connection multiplexing

### websocketproxy
**Purpose**: WebSocket proxy functionality
- Handles WebSocket connection proxying
- Maintains WebSocket protocol compliance
- Supports bidirectional communication

### forwardproxy
**Purpose**: Forward proxy implementation
- Acts as a forward proxy for client requests
- Supports HTTP CONNECT tunneling

### sshprox
**Purpose**: SSH proxy and terminal access
- Provides SSH-based proxy functionality
- Includes embedded terminal access
- Supports SSH tunneling

## Monitoring & Analytics

### statistic
**Purpose**: Statistics collection and analytics
- Collects and analyzes proxy traffic statistics
- Provides usage analytics and reporting
- Includes data visualization capabilities

### uptime
**Purpose**: Uptime monitoring system
- Monitors backend service availability
- Tracks service health and response times
- Provides uptime statistics and alerting

### netstat
**Purpose**: Network statistics and monitoring
- Collects network interface statistics
- Monitors network performance metrics
- Provides network diagnostics

## Utilities & Tools

### database
**Purpose**: Database abstraction layer
- Supports multiple database backends (BoltDB, LevelDB, etc.)
- Provides unified database interface
- Includes database utilities and migration tools

### geodb
**Purpose**: Geographic database for IP geolocation
- Maps IP addresses to geographic locations
- Provides country and city information
- Supports IPv4 and IPv6 geolocation

### netutils
**Purpose**: Network utility functions
- Implements ping, traceroute, and whois functionality
- Provides IP address utilities and validation
- Includes network diagnostic tools

### ipscan
**Purpose**: IP scanning and network discovery
- Performs IP address scanning
- Provides port scanning capabilities
- Network discovery and enumeration

### mdns
**Purpose**: Multicast DNS service discovery
- Discovers services on the local network
- Implements mDNS protocol
- Provides service advertisement

### wakeonlan
**Purpose**: Wake-on-LAN functionality
- Sends Wake-on-LAN packets to wake up devices
- Supports network device power management

## System Management

### update
**Purpose**: Update system for the application
- Manages application updates and versioning
- Provides update download and installation
- Includes rollback capabilities

### plugins
**Purpose**: Plugin system for extensibility
- Manages plugin loading and lifecycle
- Provides plugin development framework
- Supports dynamic plugin installation

### eventsystem
**Purpose**: Event system for inter-module communication
- Implements publish-subscribe pattern
- Enables loose coupling between modules
- Provides event-driven architecture

### info
**Purpose**: Information and logging utilities
- Centralized logging system
- Log viewer and management
- System information collection

## Integration & External Services

### dockerux
**Purpose**: Docker integration and container management
- Interacts with Docker daemon
- Manages containerized applications
- Provides Docker-specific utilities

### email
**Purpose**: Email functionality
- Sends email notifications
- Manages email templates and configuration

### expose
**Purpose**: Service exposure functionality
- Exposes internal services to external networks
- Manages service discovery and registration
- Provides security for exposed services

### pathrule
**Purpose**: Path-based routing rules
- Implements advanced path matching and routing
- Supports regex-based path rules
- Provides flexible routing configuration

## Development & Testing

### utils
**Purpose**: General utility functions
- Common utility functions used across modules
- Template processing and data conversion
- Testing utilities

## Module Architecture

Each module follows these design principles:

1. **Independence**: Modules are self-contained with minimal dependencies
2. **Interface-based**: Clear interfaces for inter-module communication
3. **Configuration**: Each module accepts configuration through defined structures
4. **Error Handling**: Proper error propagation and logging
5. **Testing**: Unit tests for core functionality

## Contributing

When adding new modules:

1. Create a new directory under `src/mod/`
2. Implement the module with clear interfaces
3. Add comprehensive documentation
4. Include unit tests
5. Update this README with the new module information

## Dependencies

Modules may depend on:
- Standard Go libraries
- Other Zoraxy modules (through interfaces)
- External libraries (documented in go.mod)

For more detailed information about specific modules, refer to their individual README files or source code documentation.