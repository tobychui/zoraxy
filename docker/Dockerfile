FROM docker.io/golang:alpine
# VERSION comes from the main.yml workflow --build-arg
ARG VERSION

RUN apk add --no-cache bash netcat-openbsd sudo
# Alternatives for security
RUN apk add --no-cache openssl=3.1.4-r1

RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /opt/zoraxy/config/ &&\
    mkdir -p /usr/local/bin/

COPY entrypoint.sh /opt/zoraxy/

RUN chmod -R 755 /opt/zoraxy/ &&\
    chmod +x /opt/zoraxy/entrypoint.sh

VOLUME [ "/opt/zoraxy/config/" ]

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    rm -r /opt/zoraxy/source/

RUN chmod +x /usr/local/bin/zoraxy

WORKDIR /opt/zoraxy/config/

ENV VERSION=$VERSION

ENV ARGS="-noauth=false"

ENTRYPOINT ["/opt/zoraxy/entrypoint.sh"]

HEALTHCHECK --interval=5s --timeout=5s --retries=2 CMD nc -vz 127.0.0.1 8000 || exit 1

