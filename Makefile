.PHONY: sdk-install sdk-test sdk-lint server-build ui-install ui-dev

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

# ── All ───────────────────────────────────────────────
install: sdk-install ui-install
