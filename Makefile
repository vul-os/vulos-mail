.PHONY: build test race cover vet fullstack docker-build docker-up smoke

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

# The closed-loop full-system simulation (all protocols, offline).
fullstack:
	go test ./internal/server/ -run TestFullStackSimulation -v

docker-build:
	docker build -t vulos-mail:dev .

docker-up:
	docker compose up --build

# Build image, run it, hit the JMAP session endpoint, tear down.
smoke: docker-build
	docker run -d --rm --name vulos-mail-smoke -e VULOS_ACCOUNT=alice@vulos.to -e VULOS_PASSWORD=pw -p 2080:2080 vulos-mail:dev
	@sleep 2 && curl -fsS -u alice@vulos.to:pw http://localhost:2080/jmap/session >/dev/null && echo "JMAP OK" || (docker logs vulos-mail-smoke; false)
	docker stop vulos-mail-smoke
