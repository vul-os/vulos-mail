# Multi-stage build: pure-Go, static, small runtime image.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vmail ./cmd/vmail

FROM alpine:3.20
RUN adduser -D -u 10001 vmail
COPY --from=build /out/vmail /usr/local/bin/vmail
USER vmail
ENV VMAIL_DATA_DIR=/data \
    VMAIL_MX_ADDR=:2525 \
    VMAIL_SUBMIT_ADDR=:2587 \
    VMAIL_IMAP_ADDR=:2143 \
    VMAIL_JMAP_ADDR=:2080
VOLUME ["/data"]
EXPOSE 2525 2587 2143 2080
ENTRYPOINT ["/usr/local/bin/vmail"]
