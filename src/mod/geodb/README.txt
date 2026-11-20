The embedded geoip dataset (geoipv4.csv / geoipv6.csv) is the
"IP to Country Lite" database by DB-IP, licensed under the
Creative Commons Attribution 4.0 International License (CC BY 4.0).

  IP Geolocation by DB-IP: https://db-ip.com
  License: https://creativecommons.org/licenses/by/4.0/

The license requires a link back to DB-IP.com on pages that display or
use results from the database. Keep the "IP Geolocation by DB-IP" links in
the web UI when changing pages that show geoip lookup results.

The csv files are taken from the ip-location-db project, which repackages
the DB-IP data into the "start,end,country code" format (the files without
the -num / -cidr suffix):

https://github.com/sapics/ip-location-db/releases/tag/latest
  dbip-country-ipv4.csv -> geoipv4.csv
  dbip-country-ipv6.csv -> geoipv6.csv

Run tools/update_geodb.sh to refresh the embedded files. The runtime updater
(-update_geoip flag and the scheduled updater) downloads the same files into
the external geodb folder (./conf/geodb by default).

Do not switch to the GeoLite2 (MaxMind) files due to their license terms.
