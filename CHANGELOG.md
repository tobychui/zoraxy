v2.6.5 Jul 19 2023

+ Added Import / Export-Feature 
+ Moved configurationfiles to a separate folder [#26](https://github.com/tobychui/zoraxy/issues/26)
+ Added auto-renew with ACME [#6](https://github.com/tobychui/zoraxy/issues/6)
+ Fixed Whitelistbug [#18](https://github.com/tobychui/zoraxy/issues/18)
+ Added Whois

# v2.6.4 Jun 15 2023

+ Added force TLS v1.2 above toggle
+ Added trace route
+ Added ICMP ping
+ Added special routing rules module for up-coming acme integration
+ Fixed IPv6 check bug in black/whitelist
+ Optimized UI for TCP Proxy

# v2.6.3 Jun 8 2023

+ Added X-Forwarded-Proto for automatic proxy detector
+ Split blacklist and whitelist from geodb script file
+ Optimized compile binary size
+ Added access control to TCP proxy
+ Added "invalid config detect" in up time monitor for isse [#7](https://github.com/tobychui/zoraxy/issues/7)
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
+ Added referer and file type analysis
+ Added cert expire day display
+ Moved subdomain proxy logic to dpcore
