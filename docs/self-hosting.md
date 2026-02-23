# Self-Hosting MuninnDB

MuninnDB ships as a single binary. Pre-built release binaries from GitHub Releases include an embedded ONNX model for semantic search. When building from source, the model is optional and requires `make fetch-assets` at build time. No external dependencies are required for basic operation.

---

## Option 1: Binary (recommended for development)

### 1. Download

**macOS / Linux — latest release**
```sh
# macOS arm64 (Apple Silicon)
curl -sSL https://github.com/scrypster/muninndb/releases/latest/download/muninn_darwin_arm64.tar.gz | tar -xz
sudo mv muninn /usr/local/bin/

# macOS amd64 (Intel)
curl -sSL https://github.com/scrypster/muninndb/releases/latest/download/muninn_darwin_amd64.tar.gz | tar -xz
sudo mv muninn /usr/local/bin/

# Linux amd64
curl -sSL https://github.com/scrypster/muninndb/releases/latest/download/muninn_linux_amd64.tar.gz | tar -xz
sudo mv muninn /usr/local/bin/
```

### 2. Start

```sh
muninn init    # first-time: guided setup, generates auth token, configures AI tools
muninn start   # starts the server in the background
muninn status  # verify everything is running
```

### 3. Stop

```sh
muninn stop
```

---

## Option 2: Docker

### Quick start (bundled embedder, no API key required)

```sh
docker run -d \
  --name muninndb \
  -p 8474:8474 \
  -p 8475:8475 \
  -p 8476:8476 \
  -p 8477:8477 \
  -p 8750:8750 \
  -v muninndb-data:/data \
  -e MUNINN_LOCAL_EMBED=1 \
  ghcr.io/scrypster/muninndb:latest
```

Open the Web UI: http://localhost:8476

### Docker Compose

```sh
git clone https://github.com/scrypster/muninndb
cd muninndb
docker compose up -d
```

Edit `docker-compose.yml` to configure your embedder (see [Embedder Configuration](#embedder-configuration) below).

### Build from source

```sh
git clone https://github.com/scrypster/muninndb
cd muninndb
docker build -t muninndb:local .
docker run -d --name muninndb -p 8474-8477:8474-8477 -p 8750:8750 \
  -v muninndb-data:/data muninndb:local
```

---

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8474 | TCP | MBP binary protocol (Go SDK, Python SDK) |
| 8475 | HTTP | REST API |
| 8476 | HTTP | Web UI dashboard |
| 8477 | gRPC | gRPC API |
| 8750 | HTTP | MCP — AI tool integration (Claude, Cursor, VS Code) |

---

## Embedder Configuration

MuninnDB uses embeddings for semantic search and activation. Configure with environment variables:

### Bundled (no API key, no internet)

```sh
MUNINN_LOCAL_EMBED=1
```

Uses the embedded `all-MiniLM-L6-v2` INT8 model. 384-dimensional, ~80MB model baked into the binary. Best for getting started quickly or air-gapped deployments.

### Ollama (local GPU/CPU, no API cost)

```sh
MUNINN_OLLAMA_URL=ollama://localhost:11434/nomic-embed-text
```

Install [Ollama](https://ollama.com), then:
```sh
ollama pull nomic-embed-text
```

### OpenAI

```sh
MUNINN_OPENAI_KEY=sk-...
```

Uses `text-embedding-3-small` (1536-dim). ~$0.02 per million tokens.

### Voyage AI

```sh
MUNINN_VOYAGE_KEY=pa-...
```

Uses `voyage-3` (1024-dim). High-quality retrieval, competitive pricing.

---

## Optional: LLM Enrichment

Enrichment adds summaries, keywords, and contradiction detection on top of what the cognitive engine does natively. It is not required for core functionality.

```sh
# Ollama (free, local)
MUNINN_ENRICH_URL=ollama://localhost:11434/llama3.2

# Anthropic Claude (best quality)
MUNINN_ENRICH_URL=anthropic://claude-haiku-4-5-20251001
MUNINN_ANTHROPIC_KEY=sk-ant-...

# OpenAI
MUNINN_ENRICH_URL=openai://gpt-4o-mini
MUNINN_ENRICH_API_KEY=sk-...
```

---

## Connecting AI Tools

### Automatic setup (binary install)

```sh
muninn init
```

`muninn init` detects Claude Desktop, Cursor, Windsurf, and VS Code and writes the MCP config automatically.

### Manual setup

Add to your AI tool's MCP config:

**Claude Desktop** — `~/Library/Application Support/Claude/claude_desktop_config.json`
```json
{
  "mcpServers": {
    "muninn": {
      "url": "http://localhost:8750/mcp"
    }
  }
}
```

If you enabled MCP auth (token file at `~/.muninn/mcp.token`):
```json
{
  "mcpServers": {
    "muninn": {
      "url": "http://localhost:8750/mcp",
      "headers": {
        "Authorization": "Bearer <your-token>"
      }
    }
  }
}
```

**Cursor** — `~/.cursor/mcp.json`
```json
{
  "mcpServers": {
    "muninn": {
      "url": "http://localhost:8750/mcp"
    }
  }
}
```

**VS Code** — `.vscode/mcp.json` (workspace)
```json
{
  "servers": {
    "muninn": {
      "url": "http://localhost:8750/mcp"
    }
  }
}
```

**Windsurf** — `~/.codeium/windsurf/mcp_config.json`
```json
{
  "mcpServers": {
    "muninn": {
      "url": "http://localhost:8750/mcp"
    }
  }
}
```

Restart your AI tool after editing the config.

### Verify the connection

```sh
curl http://localhost:8750/mcp/health
# → {"status":"ok"}
```

---

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `MUNINNDB_DATA` | `~/.muninn/data` | Data directory (binary) or `/data` (Docker) |
| `MUNINN_LOCAL_EMBED` | `""` | Set to `"1"` to use bundled ONNX embedder |
| `MUNINN_OPENAI_KEY` | `""` | OpenAI API key for embeddings |
| `MUNINN_OLLAMA_URL` | `""` | Ollama URL for embeddings, e.g. `ollama://localhost:11434/nomic-embed-text` |
| `MUNINN_VOYAGE_KEY` | `""` | Voyage AI key for embeddings |
| `MUNINN_ENRICH_URL` | `""` | LLM enrichment URL (optional) |
| `MUNINN_ANTHROPIC_KEY` | `""` | Anthropic API key for enrichment |
| `MUNINN_ENRICH_API_KEY` | `""` | Generic enrichment API key |
| `MUNINN_MEM_LIMIT_GB` | `4` | GOMEMLIMIT in GB |
| `MUNINN_GC_PERCENT` | `200` | GOGC tuning |
| `MUNINN_CORS_ORIGINS` | `""` | Comma-separated allowed CORS origins |

---

## Data Directory Layout

```
~/.muninn/data/         (or /data in Docker)
├── pebble/             Pebble key-value store (engrams, indices, weights)
├── wal/                Write-ahead log segments
├── auth_secret         Session signing key (auto-generated)
└── muninn.pid          Server PID (binary installs only)
~/.muninn/
└── mcp.token           MCP bearer token (auto-generated by muninn init)
```

---

## Upgrading

**Binary:**
```sh
muninn stop
# download new binary
muninn start
```

**Docker:**
```sh
docker pull ghcr.io/scrypster/muninndb:latest
docker compose up -d --pull always
```

Data in the volume is preserved across upgrades.

---

## Health Check

All services expose the same health endpoint:
```sh
curl http://localhost:8750/mcp/health
```

Returns `{"status":"ok"}` when the server is ready to accept requests.
