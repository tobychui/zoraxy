#!/bin/bash

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
cd "${SCRIPT_DIR}"/../src/mod/geodb

# Delete the old csv files
rm geoipv4.csv
rm geoipv6.csv

echo "Updating geodb csv files (DB-IP IP to Country Lite, CC BY 4.0, https://db-ip.com)"

echo "Downloading IPv4 database"
curl -fL https://github.com/sapics/ip-location-db/releases/download/latest/dbip-country-ipv4.csv -o geoipv4.csv
if [ $? -ne 0 ]; then
  echo "Failed to download IPv4 database"
  failed=true
else
  echo "Successfully downloaded IPv4 database"
fi

echo "Downloading IPv6 database"
curl -fL https://github.com/sapics/ip-location-db/releases/download/latest/dbip-country-ipv6.csv -o geoipv6.csv
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
