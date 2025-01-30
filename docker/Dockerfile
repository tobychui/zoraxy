# Stage 1: Build Zoraxy
FROM docker.io/golang:alpine AS build-zoraxy

# Install dependencies
RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /usr/local/bin/

WORKDIR /opt/zoraxy/source/

# Copy go.mod and go.sum first to leverage Docker cache
COPY ./src/go.mod ./src/go.sum ./
RUN go mod tidy

# Copy the rest of the source code
COPY ./src/ .

# Build the application
RUN go build -o /usr/local/bin/zoraxy &&\
    chmod 755 /usr/local/bin/zoraxy

# Stage 2: Build ZeroTier
FROM docker.io/ubuntu:latest AS build-zerotier

# Install dependencies
RUN mkdir -p /opt/zerotier/source/ &&\
    mkdir -p /usr/local/bin/

WORKDIR /opt/zerotier/source/

RUN apt-get update -y &&\
    apt-get install -y curl jq build-essential pkg-config clang cargo libssl-dev

# Download and build ZeroTier
RUN curl -Lo ZeroTierOne.tar.gz https://codeload.github.com/zerotier/ZeroTierOne/tar.gz/refs/tags/1.10.6 &&\
    tar -xzvf ZeroTierOne.tar.gz &&\
    cd ZeroTierOne-* &&\
    make &&\
    mv ./zerotier-one /usr/local/bin/zerotier-one &&\
    chmod 755 /usr/local/bin/zerotier-one

# Stage 3: Final image
FROM docker.io/ubuntu:latest

# Install runtime dependencies
RUN apt-get update -y &&\
    apt-get install -y bash sudo netcat-openbsd libssl-dev ca-certificates

# Copy entrypoint script
COPY --chmod=700 ./entrypoint.sh /opt/zoraxy/

# Copy built binaries from previous stages
COPY --from=build-zoraxy /usr/local/bin/zoraxy /usr/local/bin/zoraxy
COPY --from=build-zerotier /usr/local/bin/zerotier-one /usr/local/bin/zerotier-one

WORKDIR /opt/zoraxy/config/

# Set environment variables
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
ENV PORT="8000"
ENV SSHLB="false"
ENV UPDATE_GEOIP="false"
ENV VERSION="false"
ENV WEBFM="true"
ENV WEBROOT="./www"
ENV ZTAUTH=""
ENV ZTPORT="9993"

# Define volumes
VOLUME [ "/opt/zoraxy/config/" ]

# Set entrypoint
ENTRYPOINT [ "/opt/zoraxy/entrypoint.sh" ]

# Healthcheck
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD nc -vz 127.0.0.1 $PORT || exit 1

