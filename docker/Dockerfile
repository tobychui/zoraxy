FROM alpine:latest

RUN apk update && apk upgrade &&\
    apk add bash curl jq git go sudo

RUN mkdir -p /zoraxy/source/ &&\
    mkdir -p /zoraxy/config/

VOLUME [ "/zoraxy/config/" ]

COPY entrypoint.sh /zoraxy/
COPY notifier.sh /zoraxy/

RUN chmod 755 /zoraxy/ &&\
    chmod +x /zoraxy/entrypoint.sh

ENV DOCKER="2.0.0"
ENV NOTIFS="1"

ENV VERSION="latest"
ENV ARGS="-port=:8000 -noauth=false"

ENTRYPOINT ["/zoraxy/entrypoint.sh"]
