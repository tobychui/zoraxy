![](./img/title.png)

# Zoraxy

A general purpose HTTP reverse proxy and forwarding tool. Now written in Go!

### Features

- Simple to use interface with detail in-system instructions
- Reverse Proxy (HTTP/2)
  - Virtual Directory
  - WebSocket Proxy (automatic, no set-up needed) 
  - Basic Auth
  - Alias Hostnames
  - Custom Headers
  - Load Balancing
- Redirection Rules
- TLS / SSL setup and deploy
  - ACME features like auto-renew to serve your sites in http**s**
  - SNI support (and SAN certs)
  - DNS Challenge for Let's Encrypt and [these DNS providers](https://go-acme.github.io/lego/dns/)
- Blacklist / Whitelist by country or IP address (single IP, CIDR or wildcard for beginners)
- Stream Proxy (TCP & UDP)
- Integrated Up-time Monitor
- Web-SSH Terminal
- Plugin System
- Utilities
  - CIDR IP converters
  - mDNS Scanner
  - Wake-On-Lan
  - Debug Forward Proxy
  - IP Scanner
  - Port Scanner
- Others
  - Basic single-admin management mode
  - External permission management system for easy system integration
  - SMTP config for password reset
  - Dark Theme Mode

## Downloads

[Windows](https://github.com/tobychui/zoraxy/releases/latest/download/zoraxy_windows_amd64.exe)
/ [Linux (amd64)](https://github.com/tobychui/zoraxy/releases/latest/download/zoraxy_linux_amd64)
/ [Linux (arm64)](https://github.com/tobychui/zoraxy/releases/latest/download/zoraxy_linux_arm64)

For other systems or architectures, please see [Releases](https://github.com/tobychui/zoraxy/releases/latest/) 

## Getting Started

[Installing Zoraxy Reverse Proxy: Your Gateway to Efficient Web Routing](https://geekscircuit.com/installing-zoraxy-reverse-proxy-your-gateway-to-efficient-web-routing/)

Thank you for the well written and easy to follow tutorial by Reddit user [itsvmn](https://www.reddit.com/user/itsvmn/)! 
If you have no background in setting up reverse proxy or web routing, you should check this out before you start setting up your Zoraxy. 

## Build from Source

Requires Go 1.23 or higher

```bash
git clone https://github.com/tobychui/zoraxy
cd ./zoraxy/src/
go mod tidy
go build

sudo ./zoraxy -port=:8000
```

## Usage

Zoraxy provides basic authentication system for standalone mode. To use it in standalone mode, follow the instructions below for your desired deployment platform.

### Standalone Mode

Standalone mode is the default mode for Zoraxy. This allows a single account to manage your reverse proxy server just like a basic home router. This mode is suitable for new owners to homelabs or makers starting growing their web services into multiple servers. A full "Getting Started" guide can be found [here](https://github.com/tobychui/zoraxy/wiki/Getting-Started).

#### Linux

```bash
sudo ./zoraxy -port=:8000
```

#### Windows

Download the binary executable and double click the binary file to start it.

#### Raspberry Pi

The installation method is same as Linux. If you are using a Raspberry Pi 4 or newer models, pick the arm64 release. For older version of Pis, use the arm (armv6) version instead.

#### Other ARM SBCs or Android phone with Termux

The installation method is same as Linux. For other ARM SBCs, please refer to your SBC's CPU architecture and pick the one that is suitable for your device. 

#### Docker

See the [/docker](https://github.com/tobychui/zoraxy/tree/main/docker) folder for more details.

### Start Parameters

```
Usage of zoraxy:
  -autorenew int
        ACME auto TLS/SSL certificate renew check interval (seconds) (default 86400)
  -cfgupgrade
        Enable auto config upgrade if breaking change is detected (default true)
  -db string
        Database backend to use (leveldb, boltdb, auto) Note that fsdb will be used on unsupported platforms like RISCV (default "auto")
  -default_inbound_enabled
        If web server is enabled by default (default true)
  -default_inbound_port int
        Default web server listening port (default 443)
  -dev
        Use external web folder for UI development
  -docker
        Run Zoraxy in docker compatibility mode
  -earlyrenew int
        Number of days to early renew a soon expiring certificate (days) (default 30)
  -fastgeoip
        Enable high speed geoip lookup, require 1GB extra memory (Not recommend for low end devices)
  -log string
        Log folder path (default "./log")
  -mdns
        Enable mDNS scanner and transponder (default true)
  -mdnsname string
        mDNS name, leave empty to use default (zoraxy_{node-uuid}.local)
  -noauth
        Disable authentication for management interface
  -plugin string
        Plugin folder path (default "./plugins")
  -port string
        Management web interface listening port (default ":8000")
  -sshlb
        Allow loopback web ssh connection (DANGER)
  -update_geoip
        Download the latest GeoIP data and exit
  -uuid string
        sys.uuid file path (default "./sys.uuid")
  -version
        Show version of this server
  -webfm
        Enable web file manager for static web server root folder (default true)
  -webroot string
        Static web server root folder. Only allow change in start paramters (default "./www")
```

### External Permission Management Mode

If you already have an upstream reverse proxy server in place with permission management, you can use Zoraxy in noauth mode. To enable noauth mode, start Zoraxy with the following flag:

```bash
./zoraxy -noauth=true
```

> [!WARNING]
> For security reasons, you should only enable no-auth if you are running Zoraxy in a trusted environment or with another authentication management proxy in front.*

## Screenshots

![](img/screenshots/1.png)

![](img/screenshots/2.png)

More screenshots on the wikipage [Screenshots](https://github.com/tobychui/zoraxy/wiki/Screenshots)!

## FAQ

There is a wikipage with [Frequently-Asked-Questions](https://github.com/tobychui/zoraxy/wiki/FAQ---Frequently-Asked-Questions)!

## Global Area Network Controller

Moved to official plugin repo, see [ztnc](https://github.com/aroz-online/zoraxy-official-plugins/tree/main/src/ztnc) plugin

## Web SSH

Web SSH currently only supports Linux based OSes. The following platforms are supported:

- linux/amd64
- linux/arm64
- linux/armv6 (experimental)
- linux/386 (experimental)

### Loopback Connection

Loopback web SSH connections, by default, are disabled. This means that if you are trying to connect to an address like 127.0.0.1 or localhost, the system will reject your connection for security reasons. To enable loopback for testing or development purpose, use the following flags to override the loopback checking:

```bash
./zoraxy -sshlb=true
```

## Community Maintained Sections

Some section of Zoraxy are contributed by our amazing community and if you have any issues regarding those sections, it would be more efficient if you can tag them directly when creating an issue report.

- Forward Auth [@james-d-elliott](https://github.com/james-d-elliott)

  - (Legacy) Authelia Support added by [@7brend7](https://github.com/7brend7)

  - (Legacy) Authentik Support added by [@JokerQyou](https://github.com/JokerQyou)


- ACME

  - ACME integration (Looking for maintainer)

  - DNS Challenge by [@zen8841](https://github.com/zen8841)

- Docker Container List by [@eyerrock](https://github.com/eyerrock)

- Stream Proxy [@jemmy1794](https://github.com/jemmy1794)

- Change Log [@Morethanevil](https://github.com/Morethanevil)

### Looking for Maintainer

- ACME DNS Challenge Module
- Logging (including analysis & attack prevention) Module

Thank you so much for your contributions!

## Sponsor This Project

If you like the project and want to support us, please consider a donation. You can use the links below

- [tobychui (Primary author)](https://paypal.me/tobychui)
- [PassiveLemon (Docker compatibility maintainer)](https://github.com/PassiveLemon)

## License

This project is open-sourced under AGPL. I open-sourced this project so everyone can check for security issues and benefit all users. **This software is intended to be free of charge. If you have acquired this software from a third-party seller, the authors of this repository bears no responsibility for any technical difficulties assistance or support.**

