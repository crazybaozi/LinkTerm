FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY proto/ /build/proto/
COPY server/ /build/server/
WORKDIR /build/server
RUN go mod download && CGO_ENABLED=0 go build -ldflags="-s -w" -o /linkterm-server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /linkterm-server /usr/local/bin/linkterm-server
EXPOSE 8080
ENTRYPOINT ["linkterm-server"]
CMD ["-config", "/etc/linkterm/config.yaml"]
