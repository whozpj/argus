# Argus

Argus tells you when your LLM's behavior has changed — before your users do.

Wrap your existing client with one line. Run one Docker container. Get a live dashboard showing statistical drift across token counts, latency, refusal rates, and output length. Fires a Slack alert when something shifts.

Works with Anthropic, OpenAI, and any OpenAI-compatible provider. Self-hosted, no data leaves your machine.

## Quick Start

```bash
docker run -p 4000:4000 -v argus_data:/data argus/argus
```

```bash
pip install argus-sdk
```

```python
from argus_sdk import patch
patch(endpoint="http://localhost:4000")

import anthropic
client = anthropic.Anthropic()  # unchanged from here
```

Open [localhost:4000](http://localhost:4000) to see your dashboard.

## Development

Requirements: Python 3.12+, Go 1.23+, Node 20+, Docker

```bash
make sdk-install   # set up Python SDK
make server-build  # build Go server
make ui-install    # install dashboard deps
```

## Project Structure

```
sdk/        Python package (pip install argus-sdk)
server/     Go server — ingest, drift detection, SQLite
ui/         Next.js dashboard
deploy/     Dockerfile
docs/       Documentation
```
