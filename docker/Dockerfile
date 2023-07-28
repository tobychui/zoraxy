FROM docker.io/golang:1.21rc3-alpine3.18

RUN apk add --no-cache bash curl jq git sudo

RUN mkdir -p /zoraxy/source/ &&\
    mkdir -p /zoraxy/config/

VOLUME [ "/zoraxy/config/" ]

COPY entrypoint.sh /zoraxy/
COPY notifier.sh /zoraxy/

RUN chmod 755 /zoraxy/ &&\
    chmod +x /zoraxy/entrypoint.sh

ENV DOCKER="2.1.0"
ENV NOTIFS="1"

ENV VERSION="latest"
ENV ARGS="-port=:8000 -noauth=false"

ENTRYPOINT ["/zoraxy/entrypoint.sh"]
