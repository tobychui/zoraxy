FROM docker.io/golang:alpine AS build

RUN apk add --no-cache bash netcat-openbsd sudo

RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /opt/zoraxy/config/ &&\
    mkdir -p /usr/local/bin/

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    rm -r /opt/zoraxy/source/

FROM docker.io/golang:alpine AS final

RUN mkdir -p /opt/zoraxy/config/ &&\
    mkdir -p /usr/local/bin/

RUN chmod -R 770 /opt/zoraxy/

COPY --from=build /usr/local/bin/zoraxy /usr/local/bin/zoraxy

VOLUME [ "/opt/zoraxy/config/" ]

WORKDIR /opt/zoraxy/config/

ENV AUTORENEW="86400"
ENV FASTGEOIP="false"
ENV LOG="true"
ENV MDNS="true"
ENV MDNSNAME="''"
ENV NOAUTH="false"
ENV PORT="8000"
ENV SSHLB="false"
ENV VERSION="false"
ENV WEBFM="true"
ENV WEBROOT="./www"
ENV ZTAUTH="''"
ENV ZTPORT="9993"

ENTRYPOINT "zoraxy" "-docker=true" "-autorenew=${AUTORENEW}" "-fastgeoip=${FASTGEOIP}" "-log=${LOG}" "-mdns=${MDNS}" "-mdnsname=${MDNSNAME}" "-noauth=${NOAUTH}" "-port=:${PORT}" "-sshlb=${SSHLB}" "-version=${VERSION}" "-webfm=${WEBFM}" "-webroot=${WEBROOT}" "-ztauth=${ZTAUTH}" "-ztport=${ZTPORT}"

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD nc -vz 127.0.0.1 $PORT || exit 1

