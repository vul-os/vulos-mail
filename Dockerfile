# Multi-stage build: pure-Go, static, small runtime image.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vulos-mail ./cmd/vulos-mail

FROM alpine:3.20
RUN adduser -D -u 10001 vulos-mail
COPY --from=build /out/vulos-mail /usr/local/bin/vulos-mail
COPY --from=build /src/webmail /webmail
USER vulos-mail
ENV VULOS_DATA_DIR=/data \
    VULOS_WEBMAIL_DIR=/webmail \
    VULOS_MX_ADDR=:2525 \
    VULOS_SUBMIT_ADDR=:2587 \
    VULOS_IMAP_ADDR=:2143 \
    VULOS_JMAP_ADDR=:2080
VOLUME ["/data"]
EXPOSE 2525 2587 2143 2080
ENTRYPOINT ["/usr/local/bin/vulos-mail"]
