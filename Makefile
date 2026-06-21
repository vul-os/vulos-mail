.PHONY: build test race cover vet fullstack fuzz webtest e2e e2e-ext e2e-acme test-all docker-build docker-up smoke

build:
	go build -o bin/vulos-mail ./cmd/vulos-mail

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

# Comprehensive: vet + race + coverage summary.
cover:
	go test -coverprofile=cover.out ./...
	@go tool cover -func=cover.out | tail -1

# The in-process closed-loop simulation (all protocols wired together, offline).
fullstack:
	go test ./internal/server/ -run TestEndToEndAllProtocols -v

# Fuzz the untrusted-input parsers (mail/MIME, log codec, DSN, A-R stripping).
# FUZZTIME overrides the per-target duration (default 20s).
FUZZTIME ?= 20s
fuzz:
	go test ./internal/mime/   -run '^$$' -fuzz '^FuzzParse$$'            -fuzztime=$(FUZZTIME)
	go test ./internal/event/  -run '^$$' -fuzz '^FuzzDecode$$'          -fuzztime=$(FUZZTIME)
	go test ./internal/dsn/    -run '^$$' -fuzz '^FuzzBuild$$'           -fuzztime=$(FUZZTIME)
	go test ./adapters/smtp/   -run '^$$' -fuzz '^FuzzStripAuthResults$$' -fuzztime=$(FUZZTIME)

# Headless-browser tests of the webmail SPA (boots the server, drives Chrome).
webtest:
	./test/webmail/run.sh

# Full Dockerized ecosystem: private DNS + two mail servers delivering to each
# other, all protocols, plus real over-the-wire SPF/DKIM/DMARC verification.
e2e:
	./test/e2e/run.sh

# Extended Docker matrix: alternate backends (SQLite, S3/minio), rspamd spam
# scanning, hard-crash recovery, and load.
e2e-ext:
	./test/e2e/run-ext.sh

# Opt-in best-effort: real ACME cert issuance against a local Pebble CA.
e2e-acme:
	./test/e2e/run-acme.sh

# Everything runnable locally: vet, race-checked unit/integration, fuzz, then the
# Dockerized cross-server ecosystem and the extended backend/ops matrix.
test-all: vet race fuzz webtest e2e e2e-ext

docker-build:
	docker build -t vulos-mail:dev .

docker-up:
	docker compose up --build

# Build image, run it, hit the JMAP session endpoint, tear down.
smoke: docker-build
	docker run -d --rm --name vulos-mail-smoke -e VULOS_ACCOUNT=alice@vulos.to -e VULOS_PASSWORD=pw -p 2080:2080 vulos-mail:dev
	@sleep 2 && curl -fsS -u alice@vulos.to:pw http://localhost:2080/jmap/session >/dev/null && echo "JMAP OK" || (docker logs vulos-mail-smoke; false)
	docker stop vulos-mail-smoke
