#!/usr/bin/env bash

cleanup() {
  echo "Stop signal received. Shutting down..."
  kill -TERM "$(pidof zoraxy)" &> /dev/null && echo "Zoraxy stopped."
  kill -TERM "$(pidof zerotier-one)" &> /dev/null && echo "ZeroTier-One stopped."
  unlink /var/lib/zerotier-one/zerotier/
  exit 0
}

trap cleanup SIGTERM SIGINT TERM INT

update-ca-certificates && echo "CA certificates updated."
zoraxy -update_geoip=true && echo "GeoIP data updated ."


if [ "$ZEROTIER" = "true" ]; then
  if [ ! -d "/opt/zoraxy/config/zerotier/" ]; then
    mkdir -p /opt/zoraxy/config/zerotier/
  fi
  ln -s /opt/zoraxy/config/zerotier/ /var/lib/zerotier-one
  zerotier-one -d &
  zerotierpid=$!
  echo "ZeroTier daemon started."
fi

echo "Starting Zoraxy..."
zoraxy \
  -autorenew="$AUTORENEW" \
  -cfgupgrade="$CFGUPGRADE" \
  -db="$DB" \
  -docker="$DOCKER" \
  -earlyrenew="$EARLYRENEW" \
  -fastgeoip="$FASTGEOIP" \
  -mdns="$MDNS" \
  -mdnsname="$MDNSNAME" \
  -noauth="$NOAUTH" \
  -plugin="$PLUGIN" \
  -port=:"$PORT" \
  -sshlb="$SSHLB" \
  -update_geoip="$UPDATE_GEOIP" \
  -version="$VERSION" \
  -webfm="$WEBFM" \
  -webroot="$WEBROOT" \
  &

zoraxypid=$!
wait "$zoraxypid"
wait "$zerotierpid"

