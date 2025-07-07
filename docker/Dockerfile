## Build Zoraxy
FROM docker.io/golang:alpine AS build-zoraxy

RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /usr/local/bin/

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    chmod 755 /usr/local/bin/zoraxy


## Build ZeroTier
FROM docker.io/rust:1.79-alpine AS build-zerotier

RUN mkdir -p /opt/zerotier/source/ &&\
    mkdir -p /usr/local/bin/

WORKDIR /opt/zerotier/source/

RUN apk add --update --no-cache curl make gcc g++ linux-headers openssl-dev nano

RUN curl -Lo ZeroTierOne.tar.gz https://codeload.github.com/zerotier/ZeroTierOne/tar.gz/refs/tags/1.10.6 &&\
    tar -xzvf ZeroTierOne.tar.gz &&\
    cd ZeroTierOne-*/zeroidc &&\
    cargo update -p getrandom &&\
    cd .. &&\
    make -f make-linux.mk &&\
    mv ./zerotier-one /usr/local/bin/zerotier-one &&\
    chmod 755 /usr/local/bin/zerotier-one


## Main
FROM docker.io/golang:alpine

# If you build it yourself, you will need to add the example directory into the docker directory.

COPY --chmod=700 ./entrypoint.sh /opt/zoraxy/
COPY --chmod=700 ./build_plugins.sh /usr/local/bin/build_plugins

COPY --chmod=700 ./entrypoint.py /opt/zoraxy/

COPY --from=build-zerotier /usr/local/bin/zerotier-one /usr/local/bin/zerotier-one
COPY --from=build-zoraxy /usr/local/bin/zoraxy /usr/local/bin/zoraxy

RUN apk add --update --no-cache bash sudo netcat-openbsd libressl-dev openssh ca-certificates libc6-compat libstdc++ &&\
    mkdir -p /opt/zoraxy/plugin/ &&\
    echo "tun" | tee -a /etc/modules

WORKDIR /opt/zoraxy/config/

ENV ZEROTIER="false"

ENV AUTORENEW="86400"
ENV CFGUPGRADE="true"
ENV DB="auto"
ENV DOCKER="true"
ENV EARLYRENEW="30"
ENV FASTGEOIP="false"
ENV MDNS="true"
ENV MDNSNAME="''"
ENV NOAUTH="false"
ENV PLUGIN="/opt/zoraxy/plugin/"
ENV PORT="8000"
ENV SSHLB="false"
ENV UPDATE_GEOIP="false"
ENV VERSION="false"
ENV WEBFM="true"
ENV WEBROOT="./www"

VOLUME [ "/opt/zoraxy/config/" ]

LABEL com.imuslab.zoraxy.container-identifier="Zoraxy"

ENTRYPOINT [ "/opt/zoraxy/entrypoint.sh" ]

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD nc -vz 127.0.0.1 $PORT || exit 1

