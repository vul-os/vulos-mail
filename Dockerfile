# Multi-stage build: pure-Go server + Vite-built React webmail, small runtime.

# 1. Build the React + Vite + Tailwind webmail SPA into webmail/dist.
FROM node:20-alpine AS web
WORKDIR /web
COPY webmail/package.json webmail/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY webmail/ ./
RUN npm run build

# 2. Build the static Go server.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vulos-mail ./cmd/vulos-mail

FROM alpine:3.20
RUN adduser -D -u 10001 vulos-mail
COPY --from=build /out/vulos-mail /usr/local/bin/vulos-mail
COPY --from=web /web/dist /webmail
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
