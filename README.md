![](./img/title.png)

# Zoraxy

General purpose request (reverse) proxy and forwarding tool for low power devices. Now written in Go!

### Features

- Simple to use interface with detail in-system instructions
- Reverse Proxy
  
  - Subdomain Reverse Proxy
  
  - Virtual Directory Reverse Proxy
- Redirection Rules
- TLS / SSL setup and deploy
- Blacklist by country or IP address (single IP, CIDR or wildcard for beginners)
- Global Area Network Controller Web UI (ZeroTier not included)
- Integrated Up-time Monitor
- Web-SSH Terminal
- Utilities
  
  - CIDR IP converters
  - mDNS Scanner
  - IP Scanner
- Others
  - Basic single-admin management mode
  - External permission management system for easy system integration
  - SMTP config for password reset

## Build from Source
Require Go 1.20 or above

```
git clone https://github.com/tobychui/zoraxy
cd ./zoraxy/src/
go mod tidy
go build

sudo ./zoraxy -port=:8000
```

## Usage

Zoraxy provide basic authentication system for standalone mode. To use it in standalone mode, follow the instruction below for your desired deployment platform.

### Standalone Mode

Standalone mode is the default mode for Zoraxy. This allow single account to manage your reverse proxy server just like a home router. This mode is suitable for new owners for homelab or makers start growing their web services into multiple servers.

#### Linux

```bash
sudo ./zoraxy -port=:8000
```

#### Windows

Download the binary executable and double click the binary file to start it.

#### Raspberry Pi

The installation method is same as Linux. If you are using Raspberry Pi 4 or newer models, pick the arm64 release. For older version of Pis, use the arm (armv6) version instead.

#### Other ARM SBCs or Android phone with Termux

The installation method is same as Linux. For other ARM SBCs, please refer to your SBC's CPU architecture and pick the one that is suitable for your device. 

#### Docker
See the [/docker](https://github.com/tobychui/zoraxy/tree/main/docker) folder for more details

### External Permission Management Mode

If you already have a up-stream reverse proxy server in place with permission management, you can use Zoraxy in noauth mode. To enable noauth mode, start Zoraxy with the following flag

```bash
./zoraxy -noauth=true
```

*Note: For security reaons, you should only enable no-auth if you are running Zoraxy in a trusted environment or with another authentication management proxy in front.*

#### Use with ArozOS

[ArozOS ](https://arozos.com)subservice is a build in permission managed reverse proxy server. To use zoraxy with arozos, connect to your arozos host via ssh and use the following command to install zoraxy

```bash
# cd into your arozos subservice folder. Sometime it is under ~/arozos/src/subservice
cd ~/arozos/subservices
mkdir zoraxy
cd ./zoraxy

# Download the release binary from Github release
wget {binary executable link from release page}

# Set permission. Change this if required
sudo chmod 775 -R ./

# Start zoraxy to see if the downloaded arch is correct.
./zoraxy

# After the unzip done, press Ctrl + C to kill it
# Rename it to valid arozos subservice binary format
mv ./zoraxy zoraxy_linux_amd64

# If you are using SBCs with different CPU arch, use the following names
# mv ./zoraxy zoraxy_linux_arm
# mv ./zoraxy zoraxy_linux_arm64

# Restart arozos
sudo systemctl restart arozos
```

To start the module, go to System Settings > Modules > Subservice and enable it in the menu. You should be able to see a new module named "Zoraxy" pop up in the start menu.

## Screenshots

![](img/screenshots/0_1.png)

![](img/screenshots/1.png)

More screenshots on the wikipage [Screenshots](https://github.com/tobychui/zoraxy/wiki/Screenshots)!

## FAQ

There is a wikipage with [Frequently-Asked-Questions](https://github.com/tobychui/zoraxy/wiki/FAQ---Frequently-Asked-Questions)!

## Global Area Network Controller

This project also compatible with [ZeroTier](https://www.zerotier.com/). However, due to licensing issues, ZeroTier is not included in the binary. 

Assuming you already have a valid license, to use Zoraxy with ZeroTier, install ZeroTier on your host and then run Zoraxy in sudo mode (or Run As Administrator if you are on Windows). The program will automatically grab the authtoken at correct location in your host.

If you prefer not to run Zoraxy in sudo mode or you have some weird installation profile, you can also pass in the ZeroTier auth token using the following flags

```
./zoraxy -ztauth="your_zerotier_authtoken" -ztport=9993
```

The ZeroTier auth token can usually be found at ```/var/lib/zerotier-one/authtoken.secret``` or ```C:\ProgramData\ZeroTier\One\authtoken.secret```. 

This allows you to have infinite number of network members in your Global Area Network controller. For more technical details, see [here](https://docs.zerotier.com/self-hosting/network-controllers/).

## Web.SSH

Web SSH currently only support Linux based OS. The following platforms are supported

- linux/amd64
- linux/arm64
- linux/armv6 (experimental)
- linux/386 (experimental)

### Loopback Connection 

Loopback web ssh connection, by default, is disabled. This means that if you are trying to connect to address like 127.0.0.1 or localhost, the system will reject your connection due to security issues. To enable loopback for testing or development purpose, use the following flags to override the loopback checking.

```
./zoraxy -sshlb=true
```

## License

This project is open source under AGPL. I open source this project so everyone can check for security issues and benefit all users. **If your plans to use this project in commercial environment which violate the AGPL terms, please contact toby@imuslab.com for an alternative commercial license.** 

