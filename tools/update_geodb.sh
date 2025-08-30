#!/bin/bash

cd ../src/mod/geodb

# Delete the old csv files
rm geoipv4.csv
rm geoipv6.csv

echo "Updating geodb csv files"

echo "Downloading IPv4 database"
curl -f https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv4.csv -o geoipv4.csv
if [ $? -ne 0 ]; then
  echo "Failed to download IPv4 database"
  failed=true
else
  echo "Successfully downloaded IPv4 database"
fi

echo "Downloading IPv6 database"
curl -f https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv6.csv -o geoipv6.csv
if [ $? -ne 0 ]; then
  echo "Failed to download IPv6 database"
  failed=true
else
  echo "Successfully downloaded IPv6 database"
fi

if [ "$failed" = true ]; then
  echo "One or more downloads failed. Blocking exit..."
  while :; do
    read -p "Press [Ctrl+C] to exit..." input
  done
fi

