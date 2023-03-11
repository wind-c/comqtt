FROM golang:1.20 AS builder

RUN apk update
RUN apk add git

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN go build -o /app/comqtt ./cmd/single


FROM alpine

WORKDIR /
COPY --from=builder /app/comqtt .

# tcp
EXPOSE 1883

# websockets
EXPOSE 1882

# dashboard
EXPOSE 8080

ENTRYPOINT [ "/comqtt" ]
