FROM docker.io/golang:1.21rc3-alpine3.18

WORKDIR /etc/zoraxy/

VOLUME [ "/etc/zoraxy/config/" ]

COPY ./src ./

RUN mkdir -p /etc/zoraxy/bin

RUN go mod tidy 

RUN go build -o ./bin/zoraxy

WORKDIR /etc/zoraxy/config

CMD ["../bin/zoraxy"]
