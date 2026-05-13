FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
RUN mkdir -p /app/data
COPY comqtt-cluster-linux /app/
COPY config/dashboard-users.json /app/
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh
ENTRYPOINT ["/app/entrypoint.sh", "-conf", "/app/n1.conf.yml"]
