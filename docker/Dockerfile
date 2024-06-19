FROM docker.io/golang:alpine
# VERSION comes from the main.yml workflow --build-arg
ARG VERSION

RUN apk add --no-cache bash netcat-openbsd sudo

RUN mkdir -p /opt/zoraxy/source/ &&\
    mkdir -p /opt/zoraxy/config/ &&\
    mkdir -p /usr/local/bin/

RUN chmod -R 770 /opt/zoraxy/

VOLUME [ "/opt/zoraxy/config/" ]

# If you build it yourself, you will need to add the src directory into the docker directory.
COPY ./src/ /opt/zoraxy/source/

WORKDIR /opt/zoraxy/source/

RUN go mod tidy &&\
    go build -o /usr/local/bin/zoraxy &&\
    rm -r /opt/zoraxy/source/

RUN chmod 755 /usr/local/bin/zoraxy &&\
    chmod +x /usr/local/bin/zoraxy

WORKDIR /opt/zoraxy/config/

ENV VERSION=$VERSION
ENV ARGS="-noauth=false"

ENTRYPOINT "zoraxy" "-port=:8000" "-docker=true" "${ARGS}"

HEALTHCHECK --interval=5s --timeout=5s --retries=2 CMD nc -vz 127.0.0.1 8000 || exit 1

