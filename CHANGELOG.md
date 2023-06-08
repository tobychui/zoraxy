# v2.6.3 Jun 8 2023

+ Added X-Forwarded-Proto for automatic proxy detector
+ Split blacklist and whitelist from geodb script file
+ Optimized compile binary size
+ Added access control to TCP proxy
+ Added "invalid config detect" in up time monitor for isse #7
+ Fixed minor bugs in advance stats panel
+ Reduced file size of embedded materials

# v2.6.2 Jun 4 2023

+ Added advance stats operation tab
+ Added statistic reset #13
+ Added statistic export to csv and json (please use json)
+ Make subdomain clickable (not vdir) #12
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
