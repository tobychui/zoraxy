FROM alpine:latest

RUN apk update && apk upgrade &&\
    apk add bash curl jq sudo &&\
    mkdir -p /zoraxy/data/

VOLUME [ "/zoraxy/data/" ]

COPY entrypoint.sh /zoraxy/

RUN chmod +x /zoraxy/entrypoint.sh

ENV ARGS="-port=:8000 -noauth=false"

EXPOSE 80
EXPOSE 443
EXPOSE 8000

ENTRYPOINT ["/zoraxy/entrypoint.sh"]