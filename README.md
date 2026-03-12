<p align="center">
  <h1 align="center">Mutual Fund Analytics Service</h1>
  <p align="center">
    <strong>Production-ready Go backend for mutual fund performance analytics</strong>
  </p>
  <p align="center">
    <a href="#features">Features</a> •
    <a href="#quick-start">Quick Start</a> •
    <a href="#api-reference">API Reference</a> •
    <a href="#configuration">Configuration</a>
  </p>
</p>

---

## Overview

A high-performance Go backend service that integrates with [MFAPI.in](https://www.mfapi.in/) to fetch mutual fund NAV data, compute performance metrics (CAGR, Sharpe Ratio, Max Drawdown), and expose RESTful APIs for fund comparison and analytics.

---


## Important files 

```
1. ai_prompts.md : It have 5 prompts that I had used to make complete project
2. raw_cursor_logs.md : It have all the cursor logs ( Hard to read as it have so much data)
3. design_decisions.md : It have all the answer to queries related to design decision.
4. .cursor : It enforce best software development practises.

```

---


## Features

| Feature | Description |
|---------|-------------|
| **Data Pipeline** | Backfill & incremental sync with automatic retry and resumability |
| **Rate Limiting** | Multi-tier limiter (2/sec, 50/min, 300/hour) with SQLite persistence |
| **Analytics Engine** | CAGR, Rolling Returns, Max Drawdown, Volatility, Sharpe Ratio |
| **RESTful API** | 6 endpoints with JSON responses and proper error handling |
| **SQLite Storage** | WAL mode for high-performance concurrent reads |
| **Crash Recovery** | Checkpoint-based sync resumption after interruptions |

---

## Tracked Funds

**10 schemes** from **5 AMCs** across **2 categories**:

| AMC | Mid Cap Fund | Small Cap Fund |
|:----|:------------:|:--------------:|
| **ICICI Prudential** | `120505` | `120586` |
| **HDFC** | `118989` | `130503` |
| **Axis** | `119028` | `125354` |
| **SBI** | `119551` | `125497` |
| **Kotak Mahindra** | `118668` | `125494` |

> **Categories:** Equity Mid Cap Direct Growth, Equity Small Cap Direct Growth

---

## Quick Start

### Prerequisites

- **Go** 1.22 or higher
- **SQLite3** (bundled via go-sqlite3)

### Installation

```bash
# Clone repository
git clone <repository-url>
cd KreditBee-Mutual-Fund-Analysis

# Install dependencies
go mod tidy

# Build
go build -o bin/server ./cmd/server

# Run
./bin/server
```

Server starts at **http://localhost:8080**

### First-Time Setup

After starting the server, trigger a backfill sync to fetch historical NAV data:

```bash
# 1. Trigger backfill sync
curl -X POST http://localhost:8080/api/v1/sync/trigger \
  -H "Content-Type: application/json" \
  -d '{"sync_type": "backfill", "recalculate_metrics": true}'

# 2. Monitor progress
curl -s http://localhost:8080/api/v1/sync/status | jq '.data.summary'

# 3. Query fund rankings (after sync completes)
curl -s "http://localhost:8080/api/v1/funds/rank?window=1Y&sort_by=cagr" | jq .
```

### Run Tests

```bash
go test ./... -v
```

---

## Configuration

All configuration via environment variables:

| Variable | Default | Description |
|:---------|:--------|:------------|
| `SERVER_HOST` | `0.0.0.0` | Server bind address |
| `SERVER_PORT` | `8080` | Server port |
| `DATABASE_PATH` | `data/mf_analytics.db` | SQLite database file path |
| `MFAPI_BASE_URL` | `https://api.mfapi.in` | MFAPI base URL |
| `MFAPI_REQUESTS_PER_SEC` | `2` | Rate limit per second |
| `MFAPI_REQUESTS_PER_MIN` | `50` | Rate limit per minute |
| `MFAPI_REQUESTS_PER_HOUR` | `300` | Rate limit per hour |
| `MFAPI_BLOCK_DURATION` | `5m` | Block duration on violation |

---

## API Reference

**Base URL:** `http://localhost:8080`

### Endpoints Overview

| Method | Endpoint | Description |
|:------:|:---------|:------------|
| `GET` | `/health` | Health check |
| `GET` | `/api/v1/funds` | List all tracked funds |
| `GET` | `/api/v1/funds/:code` | Get fund details + latest NAV |
| `GET` | `/api/v1/funds/:code/analytics` | Get pre-computed analytics |
| `GET` | `/api/v1/funds/rank` | Rank funds by metric |
| `POST` | `/api/v1/sync/trigger` | Trigger data sync |
| `GET` | `/api/v1/sync/status` | Get sync & rate limit status |

---

### `GET /health`

Health check endpoint.

**Request:**

```bash
curl http://localhost:8080/health
```

**Response:**

```json
{
  "status": "healthy",
  "timestamp": "2026-03-12T15:47:05Z"
}
```

---

### `GET /api/v1/funds`

List all tracked mutual funds.

**Request:**

```bash
curl http://localhost:8080/api/v1/funds
```

**Response:**

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "scheme_code": 120505,
      "scheme_name": "ICICI Prudential Equity Mid Cap Direct Growth",
      "amc_name": "ICICI Prudential",
      "category_name": "Equity Mid Cap Direct Growth",
      "is_active": true,
      "created_at": "2026-03-12T15:46:59Z",
      "updated_at": "2026-03-12T15:46:59Z"
    }
  ]
}
```

---

### `GET /api/v1/funds/:code`

Get details for a specific fund including latest NAV.

**Parameters:**

| Name | Type | Required | Description |
|:-----|:-----|:--------:|:------------|
| `code` | path | Yes | Scheme code (e.g., `120505`) |

**Request:**

```bash
curl http://localhost:8080/api/v1/funds/120505
```

**Response (Success):**

```json
{
  "success": true,
  "data": {
    "fund": {
      "id": 1,
      "scheme_code": 120505,
      "scheme_name": "ICICI Prudential Equity Mid Cap Direct Growth",
      "amc_name": "ICICI Prudential",
      "category_name": "Equity Mid Cap Direct Growth"
    },
    "latest_nav": {
      "date": "2026-03-11T00:00:00Z",
      "nav": "245.6789"
    },
    "total_nav_count": 3253
  }
}
```

**Response (Not Found):**

```json
{
  "error": "not_found",
  "message": "Fund not found"
}
```

**Response (Invalid Code):**

```json
{
  "error": "invalid_parameter",
  "message": "Invalid scheme code"
}
```

---

### `GET /api/v1/funds/:code/analytics`

Get pre-computed analytics for a fund across all time windows.

**Parameters:**

| Name | Type | Required | Description |
|:-----|:-----|:--------:|:------------|
| `code` | path | Yes | Scheme code |

**Request:**

```bash
curl http://localhost:8080/api/v1/funds/120505/analytics
```

**Response:**

```json
{
  "success": true,
  "data": {
    "fund": {
      "scheme_code": 120505,
      "scheme_name": "ICICI Prudential Equity Mid Cap Direct Growth",
      "amc_name": "ICICI Prudential"
    },
    "analytics": [
      {
        "window_code": "1Y",
        "window_label": "1 Year",
        "rolling_return": "0.1523",
        "cagr": "0.1523",
        "max_drawdown": "0.1245",
        "volatility": "0.1832",
        "sharpe_ratio": "0.5043",
        "data_start_date": "2025-03-12T00:00:00Z",
        "data_end_date": "2026-03-11T00:00:00Z",
        "nav_data_point_count": 250
      },
      {
        "window_code": "3Y",
        "window_label": "3 Years",
        "rolling_return": "0.5234",
        "cagr": "0.1502",
        "max_drawdown": "0.2156",
        "volatility": "0.1945",
        "sharpe_ratio": "0.4632",
        "nav_data_point_count": 750
      }
    ]
  }
}
```

---

### `GET /api/v1/funds/rank`

Rank funds by a specific performance metric.

**Query Parameters:**

| Name | Type | Required | Default | Description |
|:-----|:-----|:--------:|:--------|:------------|
| `window` | string | No | `1Y` | Time window: `1Y`, `3Y`, `5Y`, `10Y` |
| `sort_by` | string | No | `cagr` | Metric: `cagr`, `rolling_return`, `max_drawdown`, `sharpe_ratio`, `volatility` |
| `limit` | int | No | `10` | Number of results (1-100) |

**Request:**

```bash
curl "http://localhost:8080/api/v1/funds/rank?window=1Y&sort_by=cagr&limit=5"
```

**Response:**

```json
{
  "success": true,
  "data": {
    "window": "1Y",
    "sort_by": "cagr",
    "count": 5,
    "funds": [
      {
        "rank": 1,
        "fund": {
          "scheme_code": 118989,
          "scheme_name": "HDFC Equity Mid Cap Direct Growth",
          "amc_name": "HDFC"
        },
        "analytics": {
          "cagr": "0.1823",
          "max_drawdown": "0.1102",
          "sharpe_ratio": "0.7398"
        }
      },
      {
        "rank": 2,
        "fund": {
          "scheme_code": 120505,
          "scheme_name": "ICICI Prudential Equity Mid Cap Direct Growth",
          "amc_name": "ICICI Prudential"
        },
        "analytics": {
          "cagr": "0.1523",
          "max_drawdown": "0.1245",
          "sharpe_ratio": "0.5043"
        }
      }
    ]
  }
}
```

**Response (Invalid Window):**

```json
{
  "error": "invalid_parameter",
  "message": "Invalid window code. Valid values: 1Y, 3Y, 5Y, 10Y"
}
```

---

### `POST /api/v1/sync/trigger`

Trigger data synchronization from MFAPI.

**Request Body:**

| Field | Type | Required | Description |
|:------|:-----|:--------:|:------------|
| `sync_type` | string | Yes | `backfill` (full history) or `incremental` (new data only) |
| `recalculate_metrics` | bool | No | Recalculate analytics after sync |

**Request (Backfill):**

```bash
curl -X POST http://localhost:8080/api/v1/sync/trigger \
  -H "Content-Type: application/json" \
  -d '{"sync_type": "backfill"}'
```

**Request (Incremental with Analytics):**

```bash
curl -X POST http://localhost:8080/api/v1/sync/trigger \
  -H "Content-Type: application/json" \
  -d '{"sync_type": "incremental", "recalculate_metrics": true}'
```

**Response (Success):**

```json
{
  "success": true,
  "data": {
    "message": "Sync triggered successfully",
    "sync_type": "backfill"
  }
}
```

**Response (Already Running):**

```json
{
  "error": "sync_in_progress",
  "message": "A sync operation is already in progress"
}
```

**Response (Invalid Type):**

```json
{
  "error": "invalid_request",
  "message": "Invalid request body. sync_type must be 'backfill' or 'incremental'"
}
```

---

### `GET /api/v1/sync/status`

Get synchronization status and rate limit information.

**Request:**

```bash
curl http://localhost:8080/api/v1/sync/status
```

**Response:**

```json
{
  "success": true,
  "data": {
    "is_running": true,
    "rate_limit_status": {
      "second_count": 1,
      "second_limit": 2,
      "minute_count": 15,
      "minute_limit": 50,
      "hour_count": 45,
      "hour_limit": 300,
      "is_blocked": false
    },
    "summary": {
      "total_funds": 10,
      "backfill_syncs": 6,
      "incremental_syncs": 0,
      "completed": 5,
      "failed": 0,
      "pending": 1
    },
    "funds": [
      {
        "scheme_code": 120505,
        "scheme_name": "ICICI Prudential Equity Mid Cap Direct Growth",
        "sync_type": "backfill",
        "status": "completed",
        "last_synced_at": "2026-03-12T21:17:36Z",
        "last_nav_date": "2026-03-11T00:00:00Z",
        "total_records": 3253
      },
      {
        "scheme_code": 118989,
        "scheme_name": "HDFC Equity Mid Cap Direct Growth",
        "sync_type": "backfill",
        "status": "in_progress",
        "total_records": 1500
      }
    ]
  }
}
```

---

## Analytics Metrics

| Metric | Description | Formula |
|:-------|:------------|:--------|
| **Rolling Return** | Total return over the time window | `(End NAV - Start NAV) / Start NAV` |
| **CAGR** | Compound Annual Growth Rate | `(End/Start)^(1/years) - 1` |
| **Max Drawdown** | Largest peak-to-trough decline | `max((Peak - Trough) / Peak)` |
| **Volatility** | Annualized standard deviation | `σ(daily returns) × √252` |
| **Sharpe Ratio** | Risk-adjusted return | `(CAGR - 6%) / Volatility` |

**Time Windows:** `1Y` (1 Year), `3Y` (3 Years), `5Y` (5 Years), `10Y` (10 Years)

---

## Project Structure

```
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Configuration management
│   ├── models/
│   │   └── models.go            # Domain models
│   ├── repository/
│   │   ├── sqlite.go            # Database setup & migrations
│   │   ├── fund_repository.go   # Fund & NAV data access
│   │   ├── sync_repository.go   # Sync state persistence
│   │   └── ratelimit_repository.go
│   ├── services/
│   │   ├── ratelimiter.go       # Multi-tier rate limiting
│   │   ├── mfapi_client.go      # MFAPI integration
│   │   ├── sync_service.go      # Data synchronization
│   │   ├── analytics_service.go # Performance calculations
│   │   └── fund_service.go      # Fund operations
│   └── handlers/
│       ├── router.go            # HTTP routing & middleware
│       ├── fund_handler.go      # Fund API handlers
│       └── sync_handler.go      # Sync API handlers
├── pkg/
│   └── logger/
│       └── logger.go            # Structured logging
├── go.mod
├── go.sum
├── DESIGN_DECISIONS.md
└── README.md
```

---

## Daily Operations

### Daily NAV Update

```bash
curl -X POST http://localhost:8080/api/v1/sync/trigger \
  -H "Content-Type: application/json" \
  -d '{"sync_type": "incremental", "recalculate_metrics": true}'
```

### Check Sync Progress

```bash
curl -s http://localhost:8080/api/v1/sync/status | jq '.data.summary'
```

### Get Top Performing Funds

```bash
# By CAGR (1 Year)
curl -s "http://localhost:8080/api/v1/funds/rank?window=1Y&sort_by=cagr" | jq '.data.funds[:5]'

# By Sharpe Ratio (3 Years)
curl -s "http://localhost:8080/api/v1/funds/rank?window=3Y&sort_by=sharpe_ratio" | jq '.data.funds[:5]'

# Lowest Drawdown (1 Year)
curl -s "http://localhost:8080/api/v1/funds/rank?window=1Y&sort_by=max_drawdown" | jq '.data.funds[:5]'
```

---

## License

MIT License
