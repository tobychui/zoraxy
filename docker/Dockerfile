## Build Zoraxy
FROM docker.io/golang:1.27.3-alpine3.24 AS build-zoraxy
RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /usr/local/bin/

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    chmod 755 /usr/local/bin/zoraxy

# Build entrypoint
FROM docker.io/golang:1.27.3-alpine3.24 AS build-entrypoint

COPY ./entrypoint.go /opt/zoraxy/entrypoint/

WORKDIR /opt/zoraxy/entrypoint/

RUN go build -o /usr/local/bin/zoraxy-entrypoint entrypoint.go &&\
    chmod 755 /usr/local/bin/zoraxy-entrypoint

## Build ZeroTier
FROM docker.io/rust:1.99-alpine AS build-zerotier

WORKDIR /opt/zerotier/source/

RUN apk add --update --no-cache curl make gcc g++ linux-headers openssl-dev nano &&\
    mkdir -p /opt/zerotier/source/ &&\
    mkdir -p /usr/local/bin/ &&\
    curl -Lo ZeroTierOne.tar.gz https://codeload.github.com/zerotier/ZeroTierOne/tar.gz/refs/tags/1.16.2 &&\
    tar -xzvf ZeroTierOne.tar.gz &&\
    cd ZeroTierOne-* &&\
    make -f make-linux.mk &&\
    mv ./zerotier-one /usr/local/bin/zerotier-one &&\
    chmod 755 /usr/local/bin/zerotier-one

## Main
FROM docker.io/alpine:3.24

RUN apk add --update --no-cache tzdata sudo netcat-openbsd libressl-dev openssh ca-certificates libc6-compat libstdc++ &&\
    rm -rf /sbin/apk /var/cache/apk/* /tmp/* # Simple cleanup


COPY --from=build-zerotier /usr/local/bin/zerotier-one /usr/local/bin/zerotier-one
COPY --from=build-zoraxy /usr/local/bin/zoraxy /usr/local/bin/zoraxy
COPY --from=build-entrypoint /usr/local/bin/zoraxy-entrypoint /usr/local/bin/zoraxy-entrypoint

RUN mkdir -p /opt/zoraxy/plugin/ &&\
    echo "tun" | tee -a /etc/modules # For ZeroTier

WORKDIR /opt/zoraxy/config/

ENV TZ="Etc/UTC"
ENV ZEROTIER="false"

ENV AUTORENEW="86400"
ENV CFGUPGRADE="true"
ENV DB="auto"
ENV DOCKER="true"
ENV EARLYRENEW="30"
ENV ENABLELOG="true"
ENV FASTGEOIP="false"
ENV MDNS="true"
ENV MDNSNAME="''"
ENV NOAUTH="false"
ENV PLUGIN="/opt/zoraxy/plugin/"
ENV PORT="8000"
ENV SSHLB="false"
ENV UPDATE_GEOIP="false"
ENV VERSION="false"
ENV WEBROOT="./www"

VOLUME [ "/opt/zoraxy/config/" ]

LABEL com.imuslab.zoraxy.container-identifier="Zoraxy"

ENTRYPOINT [ "/usr/local/bin/zoraxy-entrypoint" ]

HEALTHCHECK --start-period=5s --interval=15s --timeout=5s --retries=3 CMD nc -vz 127.0.0.1 $PORT || exit 1

