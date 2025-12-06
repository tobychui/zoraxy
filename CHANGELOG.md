# v3.3.0 06 Dec 2025

+ Added "Block common exploits"
+ Added "Block AI and Crawlers"
+ Added option to disable statistics
+ Added option to remove user agent
+ Show checkbox for dns challenge in zerossl by [zen8841](https://github.com/tobychui/zoraxy/commits?author=zen8841)
+ Fixed Redirection: include sub-paths checkbox [#874](https://github.com/tobychui/zoraxy/issues/874)
+ Fixed Disable load balance and do not pause upstream for 60s [#896](https://github.com/tobychui/zoraxy/issues/896)
+ Fixed ACME bug [#903](https://github.com/tobychui/zoraxy/issues/903)
+ Fixed redirection bug [#900](https://github.com/tobychui/zoraxy/issues/900)
+ smaller bugfixes
+ updated dependecies


# v3.2.9 2 Nov 2025

+ Add PKCE support with SHA256 challenge method for OAuth2 by [kjagosz](https://github.com/kjagosz) fixes [#852](https://github.com/tobychui/zoraxy/issues/852)
+ Update lego to v4.28.0 by [zen8841](https://github.com/zen8841) fixes [778](https://github.com/tobychui/zoraxy/issues/778)
+ Typo in plugins.html by [mlbarrow](mlbarrow)
+ Moved log rotation options to webmin panel
+ Supported opening tar.gz in the new log viewer
+ Added disable logging function to HTTP proxy rule for high traffic sites
+ Fixed other bugs / improvements [#855](https://github.com/tobychui/zoraxy/issues/855) [#866](https://github.com/tobychui/zoraxy/issues/866) [#867](https://github.com/tobychui/zoraxy/issues/867) [#855](https://github.com/tobychui/zoraxy/issues/856)

# v3.2.8 16 Oct 2025

+ Fixed wildcard certificate bug [#845](https://github.com/tobychui/zoraxy/issues/845) by [zen8841](https://github.com/zen8841)
+ Move function:NormalizeDomain to netutils module by [zen8841](https://github.com/zen8841)
+ Add support for Proxy Protocol V1 and V2 in streamproxy configuration by [jemmy1794](https://github.com/jemmy1794)
+ Added user selectable versions for TLS

# v3.2.7 09 Oct 2025

+ Update Sidebar CSS by [Saeraphinx](https://github.com/Saeraphinx)
+ fix restart after acme dns challenge by [jimmyGALLAND](https://github.com/jimmyGALLAND)
+ fix acme renew by [jimmyGALLAND](https://github.com/jimmyGALLAND)


# v3.2.6 (Prerelease) 16 Sep 2025

+ feat(plugins): Implement plugin API key management and authentication middleware by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ fix: Handle existing symlink in start_zerotier function by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM) [#758](https://github.com/tobychui/zoraxy/issues/758)
+ fix: panics when rewriting headers for websockets, and strange issue with logging across a month boundary by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM) [#771](https://github.com/tobychui/zoraxy/issues/771)
+ add CODEOWNERS file by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ Update lego to v4.25.2 by [zen8841](https://github.com/zen8841)
+ feat(sso): forward auth body and alternate headers by james-d-elliott [#819](https://github.com/tobychui/zoraxy/issues/819)
+ feat(sso): clear settings by [james-d-elliott](https://github.com/james-d-elliott)
+ feat(plugins): Implement event system w/ POC events by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ feature: new container environment vars by [PassiveLemon](https://github.com/PassiveLemon)
+ Update example plugins by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ feat(event system): Flesh out EventPayload interface by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ feat(plugin API): Plugin-to-plugin-comms by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ put plugin API on separate mux not protected by CSRF by [AnthonyMichaelTDM](https://github.com/AnthonyMichaelTDM)
+ fix Enable Lan and Loopback [#799](https://github.com/tobychui/zoraxy/issues/799)


# v3.2.5 20 Jul 2025


+ Added new API endpoint /api/proxy/setTlsConfig (for HTTP Proxy Editor TLS tab)
+ Refactored TLS certificate management APIs with new handlers
+ Removed redundant functions from src/cert.go and delegated to tlsCertManager
+ Code optimization in tlscert module
+ Introduced a new constant CONF_FOLDER and updated configuration storage paths (phasing out hard coded paths)
+ Updated functions to set default TLS options when missing, default to SNI
+ Added Proxy Protocol v1 support in stream proxy [jemmy1794](https://github.com/jemmy1794)
+ Fixed Proxy UI bug [jemmy1794](https://github.com/jemmy1794)
+ Fixed assign static server to localhost or all interfaces [#688](https://github.com/tobychui/zoraxy/issues/688)
+ fixed empty SSO parameters by [7brend7](https://github.com/7brend7)
+ sort list of loaded certificates by expire date by [7brend7](https://github.com/7brend7)
+ Docker hardening by [PassiveLemon](https://github.com/PassiveLemon)
+ Fixed sort by destination [#713](https://github.com/tobychui/zoraxy/issues/713)

# v3.2.4 28 Jun 2025

A big release since v3.1.9. Versions from 3.2.0 to 3.2.3 were prereleases.


+ Added Authentik support by [JokerQyou](https://github.com/tobychui/zoraxy/commits?author=JokerQyou)
+ Added pluginsystem and moved GAN and Zerotier to plugins
+ Add loopback detection [#573](https://github.com/tobychui/zoraxy/issues/573)
+ Fixed Dark theme not working with Advanced Option accordion [#591](https://github.com/tobychui/zoraxy/issues/591)
+ Update logger to include UserAgent by [Raithmir](https://github.com/Raithmir)
+ Fixed memory usage in UI [#600](https://github.com/tobychui/zoraxy/issues/600)
+ Added docker-compose.yml by [SamuelPalubaCZ](https://github.com/tobychui/zoraxy/commits?author=SamuelPalubaCZ)
+ Added more statistics for proxy hosts [#201](https://github.com/tobychui/zoraxy/issues/201) and [#608](https://github.com/tobychui/zoraxy/issues/608)
+ Fixed origin field in logs [#618](https://github.com/tobychui/zoraxy/issues/618)
+ Added FreeBSD support by Andreas Burri
+ Fixed HTTP proxy redirect [#626](https://github.com/tobychui/zoraxy/issues/626)
+ Fixed proxy handling #629](https://github.com/tobychui/zoraxy/issues/629)
+ Move Scope ID handling into CIDR check by [Nirostar](https://github.com/tobychui/zoraxy/commits?author=Nirostar)
+ Prevent the browser from filling the saved Zoraxy login account by [WHFo](https://github.com/tobychui/zoraxy/commits?author=WHFo)
+ Added port number and http proto to http proxy list link
+ Fixed headers for authelia by [james-d-elliott](https://github.com/tobychui/zoraxy/commits?author=james-d-elliott)
+ Refactored docker container list and UI improvements by [eyerrock](https://github.com/tobychui/zoraxy/commits?author=eyerrock)
+ Refactored Dockerfile by [PassiveLemon](https://github.com/tobychui/zoraxy/commits?author=PassiveLemon)
+ Added new HTTP proxy UI
+ Added inbound host name edit function
+ Added static web server option to disable listen to all interface
+ Merged SSO implementations (Oauth2) [#649](https://github.com/tobychui/zoraxy/pull/649)
+ Merged forward-auth optimization [#692(https://github.com/tobychui/zoraxy/pull/692)
+ Optimized SSO UI
+ Refactored docker image workflows by [PassiveLemon](https://github.com/tobychui/zoraxy/commits?author=PassiveLemon)
+ Added disable chunked transfer encoding checkbox (for upstreams that uses legacy HTTP implementations)
+ Bug fixes [#694](https://github.com/tobychui/zoraxy/issues/694), [#659](https://github.com/tobychui/zoraxy/issues/659) by [jemmy1794](https://github.com/tobychui/zoraxy/commits?author=jemmy1794), [#695](https://github.com/tobychui/zoraxy/issues/695)

# v3.1.9 1 Mar 2025

+ Fixed netstat underflow bug
+ Fixed origin picker cookie bug [#550](https://github.com/tobychui/zoraxy/issues/550)
+ Added prototype plugin system
+ Added plugin examples
+ Added notice for build-in Zerotier network controller deprecation (and will be moved to plugins)
+ Added country code display for quickban list [#247](https://github.com/tobychui/zoraxy/issues/247)
+ Removed passive load balancer and default to active lb only [#554](https://github.com/tobychui/zoraxy/issues/554)


# v3.1.8 16 Feb 2025

+ Exposed timeout value from dpcore to UI
+ Added active load balancing (if uptime monitor is enabled on that rule)
+ Re-factorized io stats and remove dependencies over wmic by [eyerrock](https://github.com/eyerrock)
+ Removed SMTP input validation [#497](https://github.com/tobychui/zoraxy/issues/497)
+ Fixed sticky session bug
+ Fixed passive load balancer bug
+ Fixed dockerfile bug by [PassiveLemon](https://github.com/PassiveLemon)

# v3.1.7 08 Feb 2025

+ Merged and added new tagging system for HTTP Proxy rules [by @adoolaard](https://github.com/adoolaard)
+ Added inline editing for redirection rules [#510](https://github.com/tobychui/zoraxy/issues/510)
+ Added uptime monitor status dot detail info (now clickable) [#467](https://github.com/tobychui/zoraxy/issues/467)
+ Added close connection support to port 80 listener [#405](https://github.com/tobychui/zoraxy/issues/450)
+ Optimized port collision check on startup
+ Optimized dark theme color scheme (Free consultation by 3S Design studio)
+ Fixed capital letter rule unable to delete bug [#507](https://github.com/tobychui/zoraxy/issues/507)
+ Fixed docker statistic not save bug [by @PassiveLemon](https://github.com/PassiveLemon) [#505](https://github.com/tobychui/zoraxy/issues/505)


# v3.1.6 31 Dec 2024


+ Exposed log file, sys.uuid and static web server path to start flag (customizable conf and sys.db path is still wip)
+ Optimized connection close implementation
+ Added toggle for uptime monitor
+ Added optional copy HTTP custom headers to websocket connection [#444](https://github.com/tobychui/zoraxy/issues/444)

# v3.1.5 28 Dec 2024

+ Fixed hostname case sensitive bug [#435](https://github.com/tobychui/zoraxy/issues/435)
+ Fixed ACME table too wide css bug [#422](https://github.com/tobychui/zoraxy/issues/422)
+ Fixed HSTS toggle button bug [#415](https://github.com/tobychui/zoraxy/issues/415)
+ Fixed slow GeoIP resolve mode concurrent r/w bug [#401](https://github.com/tobychui/zoraxy/issues/401)
+ Added close connection as default site option [#430](https://github.com/tobychui/zoraxy/issues/430)
+ Added experimental authelia support [#384](https://github.com/tobychui/zoraxy/issues/384)
+ Added custom header support to websocket [#426](https://github.com/tobychui/zoraxy/issues/426)
+ Added levelDB as database implementation (not currently used)
+ Added external GeoIP db loading support
+ Restructured a lot of modules

# v3.1.4 24 Nov 2024

+ **Added Dark Theme Mode** [#390](https://github.com/tobychui/zoraxy/issues/390) [#82](https://github.com/tobychui/zoraxy/issues/82)
+ Added an auto sniffer for self-signed certificates
+ Added robots.txt to the project
+ Introduced an EU wrapper in the front-end for automatic registration of 26 countries [#378](https://github.com/tobychui/zoraxy/issues/378)
+ Moved all hard-coded values to a dedicated def.go file
+ Fixed a panic issue occurring on unsupported platform exits
+ Integrated fixes for SSH proxy and Docker snippet updates [#330](https://github.com/tobychui/zoraxy/issues/330) [#348](https://github.com/tobychui/zoraxy/issues/348)
+ **Changed the default listening port to 443 and enable TLS by default**
+ Optimized GeoIP database slow-search mode CPU usage


# v3.1.3 12 Nov 2024

+ Fixed a critical security bug [CVE-2024-52010](https://github.com/advisories/GHSA-7hpf-g48v-hw3j)

# v3.1.2 03 Nov 2024

+ Added auto start port 80 listener on acme certificate generator
+ Added polling interval and propagation timeout option in ACME module [#300](https://github.com/tobychui/zoraxy/issues/300)
+ Added support for custom header variables [#318](https://github.com/tobychui/zoraxy/issues/318)
+ Added support for X-Remote-User 
+ Added port scanner [#342](https://github.com/tobychui/zoraxy/issues/342)
+ Optimized code base for stream proxy and config file storage [#320](https://github.com/tobychui/zoraxy/issues/320)
+ Removed sorting on cert list
+ Fixed request certificate button bug 
+ Fixed cert auto renew logic [#316](https://github.com/tobychui/zoraxy/issues/316)
+ Fixed unable to remove new stream proxy bug
+ Fixed many other minor bugs  [#328](https://github.com/tobychui/zoraxy/issues/328) [#297](https://github.com/tobychui/zoraxy/issues/297)
+ Added more code to SSO system (disabled in release)


# v3.1.1. 09 Sep 2024

+ Updated country name in access list [#287](https://github.com/tobychui/zoraxy/issues/287)
+ Added tour for basic operations
+ Updated acme log to system wide logger implementation
+ Fixed path traversal in file manager [#274](https://github.com/tobychui/zoraxy/issues/274)
+ Removed Proxmox debug code
+ Fixed trie tree implementations

**Thanks to all contributors**

+ Fix existing containers list in docker popup [7brend7](https://github.com/tobychui/zoraxy/issues?q=is%3Apr+author%3A7brend7)
+ Fix network I/O chart not rendering [JokerQyou](https://github.com/tobychui/zoraxy/issues?q=is%3Apr+author%3AJokerQyou)
+ Fix typo remvoeClass to removeClass [Aahmadsyamim](https://github.com/tobychui/zoraxy/issues?q=is%3Apr+author%3Aahmadsyamim)
+ Updated weighted random upstream implementation [bouroo](https://github.com/tobychui/zoraxy/issues?q=is%3Apr+author%3Abouroo)

# v3.1.0 31 Jul 2024

+ Updated log viewer with filter and auto refresh [#243](https://github.com/tobychui/zoraxy/issues/243)
+ Fixed csrf vulnerability [#267](https://github.com/tobychui/zoraxy/issues/267)
+ Fixed promox issue
+ Fixed status code bug in upstream log [#254](https://github.com/tobychui/zoraxy/issues/254)
+ Added host overwrite and hop-by-hop header remover
+ Added early renew days settings [#256](https://github.com/tobychui/zoraxy/issues/256)
+ Updated make file to force no CGO in cicd process
+ Fixed bug in updater
+ Fixed wildcard certificate renew bug [#249](https://github.com/tobychui/zoraxy/issues/249)
+ Added certificate download function [#227](https://github.com/tobychui/zoraxy/issues/227)

# v3.0.9 16 Jul 2024

+ Added certificate download [#227](https://github.com/tobychui/zoraxy/issues/227)
+ Updated netcup timeout value [#231](https://github.com/tobychui/zoraxy/issues/231)
+ Updated geoip db
+ Removed debug print from log viewer
+ Upgraded netstat log printing to new log formatter
+ Improved update module implementation

# v3.0.8 15 Jul 2024

+ Added apache style logging mechanism (and build-in log viewer) [#218](https://github.com/tobychui/zoraxy/issues/218)
+ Fixed keep alive flushing issues [#235](https://github.com/tobychui/zoraxy/issues/235)
+ Added multi-upstream supports [#100](https://github.com/tobychui/zoraxy/issues/100)
+ Added stick session load balancer
+ Added weighted random load balancer
+ Added domain cleaning logic to domain / IP input fields
+ Added HSTS "include subdomain" auto injector
+ Added work-in-progress SSO / Oauth Server UI
+ Fixed uptime monitor not updating on proxy rule change bug
+ Optimized UI for create new proxy rule
+ Removed service expose proxy feature

# v3.0.7 20 Jun 2024

+ Fixed redirection enable bug [#199](https://github.com/tobychui/zoraxy/issues/199)
+ Fixed header tool user agent rewrite sequence
+ Optimized rate limit UI
+ Added HSTS and Permission Policy Editor [#163](https://github.com/tobychui/zoraxy/issues/163)
+ Docker UX optimization start parameter `-docker`
+ Docker container selector implementation for conditional compilations for Windows

From contributors:

+ Add Rate Limits Limits to Zoraxy fixes [185](https://github.com/tobychui/zoraxy/issues/185) by [Kirari04](https://github.com/Kirari04)
+ Add docker containers list to set rule by [7brend7](https://github.com/7brend7) [PR202](https://github.com/tobychui/zoraxy/pull/202)

# v3.0.6 10 Jun 2024

+ Added fastly_client_ip to X-Real-IP auto rewrite
+ Added atomic accumulator to TCP proxy
+ Added white logo for future dark theme
+ Added multi selection for white / blacklist [#176](https://github.com/tobychui/zoraxy/issues/176)
+ Moved custom header rewrite to dpcore
+ Restructure dpcore header rewrite sequence
+ Added advance custom header settings (zoraxy to upstream and zoraxy to downstream mode)
+ Added header remove feature
+ Removed password requirement for SMTP [#162](https://github.com/tobychui/zoraxy/issues/162) [#80](https://github.com/tobychui/zoraxy/issues/80) 
+ Restructured TCP proxy into Stream Proxy (Support both TCP and UDP) [#147](https://github.com/tobychui/zoraxy/issues/147)
+ Added stream proxy auto start [#169](https://github.com/tobychui/zoraxy/issues/169)
+ Optimized UX for reminding user to click Apply after port change
+ Added version number to footer [#160](https://github.com/tobychui/zoraxy/issues/160)

From contributors:

+ Fixed missing / unnecessary error check [PR187](https://github.com/tobychui/zoraxy/pull/187) by [Kirari04](https://github.com/Kirari04)

# v3.0.5 May 26 2024


+ Optimized uptime monitor error message [#121](https://github.com/tobychui/zoraxy/issues/121)
+ Optimized detection logic for internal proxy target and header rewrite condition for HTTP_HOST [#164](https://github.com/tobychui/zoraxy/issues/164)
+ Fixed ovh DNS challenge provider form generator bug [#161](https://github.com/tobychui/zoraxy/issues/161)
+ Added permission policy module (not enabled)
+ Added single-use cookiejar to uptime monitor request client to handle cookie issues on some poorly written back-end server [#149](https://github.com/tobychui/zoraxy/issues/149)


# v3.0.4 May 18 2024

## This release tidied up the contribution by [Teifun2](https://github.com/Teifun2) and added a new way to generate DNS challenge based certificate (e.g. wildcards) from Let's Encrypt without changing any environment variables. This also fixes a few previous ACME module EAB settings bug related to concurrent save.

You can find the DNS challenge settings under TLS / SSL > ACME snippet > Generate New Certificate > (Check the "Use a DNS Challenge" checkbox)

+ Optimized DNS challenge implementation [thanks to Teifun2](https://github.com/Teifun2) / Issues [#49](https://github.com/tobychui/zoraxy/issues/49) [#79](https://github.com/tobychui/zoraxy/issues/79)
+ Removed dependencies on environment variable write and keep all data contained
+ Fixed panic on loading certificate generated by Zoraxy v2
+ Added automatic form generator for DNS challenge / providers
+ Added CA name default value
+ Added code generator for acmedns module (storing the DNS challenge provider contents extracted from lego)
+ Fixed ACME snippet "Obtain Certificate" concurrent issues in save EAB and DNS credentials


# v3.0.3 Apr 30 2024
## Breaking Change

For users using SMTP with older versions, you might need to update the settings by moving the domains (the part after @ in the username and domain setup field) into the username field.

+ Updated SMTP UI for non email login username [#129](https://github.com/tobychui/zoraxy/issues/129)
+ Fixed ACME cert store reload after cert request [#126](https://github.com/tobychui/zoraxy/issues/126)
+ Fixed default rule not applying to default site when default site is set to proxy target [#130](https://github.com/tobychui/zoraxy/issues/130)
+ Fixed blacklist-ip not working with CIDR bug
+ Fixed minor vdir bug in tailing slash detection and redirect logic
+ Added custom mdns name support (-mdnsname flag)
+ Added LAN tag in statistic [#131](https://github.com/tobychui/zoraxy/issues/131)


# v3.0.2 Apr 24 2024

+ Added alias for HTTP proxy host names [#76](https://github.com/tobychui/zoraxy/issues/76)
+ Added separator support for create new proxy rules (use "," to add alias when creating new proxy rule)
+ Added HTTP proxy host based access rules [#69](https://github.com/tobychui/zoraxy/issues/69)
+ Added EAD Configuration for ACME (by [yeungalan](https://github.com/yeungalan)) [#45](https://github.com/tobychui/zoraxy/issues/45)
+ Fixed bug for bypassGlobalTLS endpoint do not support basic-auth
+ Fixed panic due to empty domain field in json config [#120](https://github.com/tobychui/zoraxy/issues/120)
+ Removed dependencies on management panel css for online font files

# v3.0.1 Apr 04 2024

## Bugfixupdate for big release of V3, read update notes from V3 if you are still on V2

+ Added regex support for redirect (slow, don't use it unless you really needs it) [#42](https://github.com/tobychui/zoraxy/issues/42)
+ Added new dpcore implementations for faster proxy speed
+ Added support for CF-Connecting-IP to X-Real-IP auto rewrite [#114](https://github.com/tobychui/zoraxy/issues/114)
+ Added enable / disable of HTTP proxy rules in runtime via slider [#108](https://github.com/tobychui/zoraxy/issues/108)
+ Added better 404 page
+ Added option to bypass websocket origin check [#107](https://github.com/tobychui/zoraxy/issues/107)
+ Updated project homepage design
+ Fixed recursive port detection logic
+ Fixed UserAgent in resp bug
+ Updated minimum required Go version to v1.22 (Notes: Windows 7 support is dropped) [#112](https://github.com/tobychui/zoraxy/issues/112)


# v3.0.0 Feb 18 2024

## IMPORTANT: V3 is a big rewrite and it is incompatible with V2! There is NO migration, if you want to stay on V2, please use V2 branch!

+ Added comments for whitelist [#97](https://github.com/tobychui/zoraxy/issues/97)
+ Added force-renew for certificates [#92](https://github.com/tobychui/zoraxy/issues/92)
+ Added automatic cert pick for multi-host certs (SNI)
+ Renamed .crt to .pem for cert store
+ Added best-fit selection for wildcard matching rules
+ Added x-proxy-by header / Added X-real-Ip header [#93](https://github.com/tobychui/zoraxy/issues/93)
+ Added Development Mode (Cache-Control: no-store)
+ Updated utm timeout to 10 seconds instead of 90
+ Added "Add controller as member" feature to Global Area Network editor
+ Added custom header
+ Deprecated aroz subservice support
+ Updated visuals, improving logical structure, less depressing colors [#95](https://github.com/tobychui/zoraxy/issues/95)
+ Added virtual directory into host routing object (each host now got its own sets of virtual directories)
+ Added support for wildcard host names (e.g. *.example.com)
+ Added best-fit selection for wildcard matching rules (e.g. *.a.example.com > *.example.com in routing)
+ Generalized root and hosts routing struct (no more conversion between runtime & save record object
+ Added "Default Site" to replace "Proxy Root" interface
+ Added Redirect & 404 page for "Default Site"


# v2.6.8 Nov 25 2023

+ Added opt-out for subdomains for global TLS settings: See [release notes](https://github.com/tobychui/zoraxy/releases/tag/2.6.8)
+ Optimized subdomain / vdir editing interface
+ Added system-wide logger (Work in progress)
+ Fixed issue for uptime monitor bug [#77](https://github.com/tobychui/zoraxy/issues/77)
+ Changed default static web port to 5487 (prevent already in use)
+ Added automatic HTTP/2 to TLS mode
+ Bug fix for webserver autostart [67](https://github.com/tobychui/zoraxy/issues/67)

# v2.6.7 Sep 26 2023

+ Added Static Web Server function [#56](https://github.com/tobychui/zoraxy/issues/56)
+ Web Directory Manager (see static webserver tab)
+ Added static web server and black / whitelist template [#38](https://github.com/tobychui/zoraxy/issues/38)
+ Added default / preferred CA features for ACME [#47](https://github.com/tobychui/zoraxy/issues/47)
+ Optimized TLS/SSL page and added dedicated section for ACME related features
+ Bugfixes [#61](https://github.com/tobychui/zoraxy/issues/61) [#58](https://github.com/tobychui/zoraxy/issues/58)

# v2.6.6 Aug 30 2023

+ Added basic auth editor custom exception rules 
+ Fixed redirection bug under another reverse proxy and Apache location headers [#39](https://github.com/tobychui/zoraxy/issues/39)
+ Optimized memory usage (from 1.2GB to 61MB for low speed geoip lookup) [#52](https://github.com/tobychui/zoraxy/issues/52)
+ Added unset subdomain custom redirection feature [#46](https://github.com/tobychui/zoraxy/issues/46)
+ Fixed potential security issue in satori/go.uuid [#55](https://github.com/tobychui/zoraxy/issues/55)
+ Added custom ACME feature in backend, thx [@daluntw](https://github.com/daluntw)
+ Added bypass TLS check for custom acme server, thx [@daluntw](https://github.com/daluntw)
+ Introduce new start parameter `-fastgeoip=true`: see [release notes](https://github.com/tobychui/zoraxy/releases/tag/2.6.6)

# v2.6.5.1 Jul 26 2023

+ Patch on memory leaking for Windows netstat module (do not effect any of the previous non Windows builds)
+ Fixed potential memory leak in ACME handler logic
+ Added "Do you want to get a TLS certificate for this subdomain?" dialogue when a new subdomain proxy rule is created

# v2.6.5 Jul 19 2023

+ Added Import / Export-Feature 
+ Moved configuration files to a separate folder [#26](https://github.com/tobychui/zoraxy/issues/26)
+ Added auto-renew with ACME [#6](https://github.com/tobychui/zoraxy/issues/6)
+ Fixed Whitelistbug [#18](https://github.com/tobychui/zoraxy/issues/18)
+ Added Whois

# v2.6.4 Jun 15 2023

+ Added force TLS v1.2 above toggle
+ Added trace route
+ Added ICMP ping
+ Added special routing rules module for up-coming ACME integration
+ Fixed IPv6 check bug in black/whitelist
+ Optimized UI for TCP Proxy

# v2.6.3 Jun 8 2023

+ Added X-Forwarded-Proto for automatic proxy detector
+ Split blacklist and whitelist from geodb script file
+ Optimized compile binary size
+ Added access control to TCP proxy
+ Added "invalid config detect" in up time monitor for issue [#7](https://github.com/tobychui/zoraxy/issues/7)
+ Fixed minor bugs in advance stats panel
+ Reduced file size of embedded materials

# v2.6.2 Jun 4 2023

+ Added advance stats operation tab
+ Added statistic reset [#13](https://github.com/tobychui/zoraxy/issues/13)
+ Added statistic export to csv and json (please use json)
+ Make subdomain clickable (not vdir) [#12](https://github.com/tobychui/zoraxy/issues/12)
+ Added TCP Proxy
+ Updates SMTP setup UI to make it more straight forward to setup

# v2.6.1 May 31 2023

+ Added reverse proxy TLS skip verification
+ Added basic auth
+ Edit proxy settings
+ Whitelist
+ TCP Proxy (experimental)
+ Info (Utilities page)

# v2.6 May 27 2023

+ Basic auth
+ Support TLS verification skip (for self signed certs)
+ Added trend analysis
+ Added referrer and file type analysis
+ Added cert expire day display
+ Moved subdomain proxy logic to dpcore
