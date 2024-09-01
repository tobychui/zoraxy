FROM docker.io/golang:alpine AS build

RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /usr/local/bin/

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    chmod 755 /usr/local/bin/zoraxy

FROM docker.io/alpine:latest

WORKDIR /opt/zoraxy/source/

RUN apk add --no-cache bash netcat-openbsd sudo &&\
    wget https://dl-cdn.alpinelinux.org/alpine/v3.17/community/x86_64/zerotier-one-1.10.2-r0.apk &&\
    apk add --no-cache zerotier-one-1.10.2-r0.apk &&\
    rm -r /opt/zoraxy/source/

COPY --from=build /usr/local/bin/zoraxy /usr/local/bin/zoraxy
COPY --chmod=700 ./entrypoint.sh /opt/zoraxy/

WORKDIR /opt/zoraxy/config/

ENV ZEROTIER="false"

ENV AUTORENEW="86400"
ENV CFGUPGRADE="true"
ENV DOCKER="true"
ENV EARLYRENEW="30"
ENV FASTGEOIP="false"
ENV MDNS="true"
ENV MDNSNAME="''"
ENV NOAUTH="false"
ENV PORT="8000"
ENV SSHLB="false"
ENV VERSION="false"
ENV WEBFM="true"
ENV WEBROOT="./www"
ENV ZTAUTH=""
ENV ZTPORT="9993"

VOLUME [ "/opt/zoraxy/config/", "/var/lib/zerotier-one/" ]

ENTRYPOINT [ "/opt/zoraxy/entrypoint.sh" ]

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD nc -vz 127.0.0.1 $PORT || exit 1

