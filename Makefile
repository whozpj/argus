.PHONY: sdk-install sdk-test sdk-lint server-build server-test ui-install ui-dev ui-build ui-typecheck docker-build test install

# ── SDK ───────────────────────────────────────────────
sdk-install:
	cd sdk && python3 -m venv .venv && .venv/bin/pip install -e ".[dev]" -q

sdk-test:
	cd sdk && .venv/bin/pytest

sdk-lint:
	cd sdk && .venv/bin/ruff check argus_sdk

# ── Server ────────────────────────────────────────────
server-build:
	cd server && go build -o bin/argus ./cmd/...

server-test:
	cd server && go test ./...

# ── UI ────────────────────────────────────────────────
ui-install:
	cd ui && npm install

ui-dev:
	cd ui && npm run dev

ui-build:
	cd ui && npm run build

ui-typecheck:
	cd ui && npx tsc --noEmit

# ── Docker ────────────────────────────────────────────
docker-build:
	docker build -f deploy/Dockerfile -t argus .

# ── All ───────────────────────────────────────────────
test: sdk-test server-test
	@echo "All tests passed."

install: sdk-install ui-install
