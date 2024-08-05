FROM golang:1.21-alpine3.20 AS builder

RUN apk update
RUN apk add git

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN go build -o /app/comqtt ./cmd/single
RUN go build -o /app/comqtt-cluster ./cmd/cluster

FROM alpine

WORKDIR /
COPY --from=builder /app/comqtt /app/comqtt-cluster ./

ENTRYPOINT [ "/comqtt" ]
