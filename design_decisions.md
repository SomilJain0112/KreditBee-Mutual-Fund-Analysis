<p align="center">
  <h1 align="center">Design Decisions</h1>
  <p align="center">
    <strong>Architecture, patterns, and trade-offs explained</strong>
  </p>
</p>

---

## Table of Contents

- [Architecture](#architecture)
- [Rate Limiting](#rate-limiting)
- [Data Pipeline](#data-pipeline)
- [Analytics Engine](#analytics-engine)
- [API Design](#api-design)
- [Performance](#performance)
- [Error Handling](#error-handling)
- [Testing](#testing)
- [Future Improvements](#future-improvements)

---

## Architecture

### Layered Architecture Pattern

```
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Layer (Gin)                       │
│                    handlers/*.go                            │
├─────────────────────────────────────────────────────────────┤
│                     Service Layer                           │
│     fund_service.go │ sync_service.go │ analytics_service   │
├─────────────────────────────────────────────────────────────┤
│                    Repository Layer                         │
│   fund_repository.go │ sync_repository.go │ ratelimit_repo  │
├─────────────────────────────────────────────────────────────┤
│                     SQLite + WAL                            │
└─────────────────────────────────────────────────────────────┘
```

| Layer | Responsibility | Files |
|:------|:---------------|:------|
| **Handlers** | HTTP parsing, validation, response formatting | `handlers/*.go` |
| **Services** | Business logic, orchestration | `services/*.go` |
| **Repository** | Data access, queries | `repository/*.go` |

**Why This Pattern?**
- Clear separation of concerns
- Unidirectional flow prevents circular dependencies
- Each layer testable in isolation
- Easy to swap implementations (e.g., different database)

---

### Database: SQLite with WAL

| Aspect | Choice | Rationale |
|:-------|:-------|:----------|
| **Database** | SQLite | Single-file, no server needed, sufficient for 10 funds |
| **Journal Mode** | WAL | Concurrent reads during writes |
| **Sync Mode** | NORMAL | Balance between durability and performance |

**SQLite Configuration:**

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = 2000;
PRAGMA foreign_keys = ON;
```

**Trade-offs:**
| Pros | Cons |
|:-----|:-----|
| Zero configuration | Not horizontally scalable |
| Single file backup | Limited concurrent writes |
| Fast reads with WAL | No built-in replication |

---

### HTTP Framework: Gin

| Feature | Benefit |
|:--------|:--------|
| Radix tree routing | High performance routing |
| Built-in validation | `go-playground/validator` integration |
| JSON optimization | Uses `json-iterator` internally |
| Recovery middleware | Panic recovery built-in |

---

## Rate Limiting

### Multi-Tier Strategy

```
┌─────────────────────────────────────────┐
│           Rate Limit Tiers              │
├─────────────────────────────────────────┤
│  Per Second:  2 requests/sec            │
│  Per Minute:  50 requests/min           │
│  Per Hour:    300 requests/hour         │
│  Block Time:  5 minutes on violation    │
└─────────────────────────────────────────┘
```

### Implementation Details

| Component | Description |
|:----------|:------------|
| **In-memory counters** | Fast access for rate checking |
| **SQLite persistence** | State survives restarts |
| **Automatic window reset** | Counters reset at window boundaries |
| **Blocking mechanism** | 5-minute block when limits exceeded |

**Why Persistence?**
- Rate limit state survives server restarts
- Prevents abuse via restart attacks
- Accurate tracking across deployments

**State Table:**

```sql
CREATE TABLE rate_limit_state (
    id           INTEGER PRIMARY KEY,
    window_type  TEXT NOT NULL,      -- 'second', 'minute', 'hour'
    window_start DATETIME NOT NULL,
    count        INTEGER DEFAULT 0,
    blocked_at   DATETIME,
    UNIQUE(window_type, window_start)
);
```

---

## Data Pipeline

### Sync Modes

| Mode | Use Case | Behavior |
|:-----|:---------|:---------|
| **Backfill** | Initial setup | Fetches all historical NAV data |
| **Incremental** | Daily updates | Fetches only new data since last sync |

### Resumability

```
┌─────────────────────────────────────────────────────────────┐
│                    Sync Flow                                │
├─────────────────────────────────────────────────────────────┤
│  1. Fetch NAV data from MFAPI                               │
│  2. Process in batches (default: 10 records)                │
│  3. Save resume pointer after each batch                    │
│  4. On interruption → restart from last pointer             │
└─────────────────────────────────────────────────────────────┘
```

**Sync State Schema:**

```sql
CREATE TABLE sync_state (
    fund_id        INTEGER NOT NULL,
    sync_type      TEXT NOT NULL,       -- 'backfill' or 'incremental'
    status         TEXT NOT NULL,       -- 'pending', 'in_progress', 'completed', 'failed', 'paused'
    last_synced_at DATETIME,
    last_nav_date  DATE,
    total_records  INTEGER DEFAULT 0,
    error_message  TEXT,
    retry_count    INTEGER DEFAULT 0,
    resume_pointer TEXT,                -- Batch index for resumption
    UNIQUE(fund_id, sync_type)
);
```

**Why Resumability Matters:**
- MFAPI rate limits can cause long sync operations
- Server restarts shouldn't lose progress
- Graceful handling of network interruptions

---

## Analytics Engine

### Metrics Calculated

| Metric | Formula | Description |
|:-------|:--------|:------------|
| **Rolling Return** | `(EndNAV - StartNAV) / StartNAV` | Total return over period |
| **CAGR** | `(EndNAV/StartNAV)^(1/years) - 1` | Annualized growth rate |
| **Max Drawdown** | `max((Peak - Trough) / Peak)` | Largest decline from peak |
| **Volatility** | `σ(daily_returns) × √252` | Annualized standard deviation |
| **Sharpe Ratio** | `(CAGR - 0.06) / Volatility` | Risk-adjusted return |

### Time Windows

| Code | Days | Use Case |
|:-----|:-----|:---------|
| `1Y` | 365 | Short-term performance |
| `3Y` | 1,095 | Medium-term track record |
| `5Y` | 1,825 | Long-term consistency |
| `10Y` | 3,650 | Full market cycle analysis |

### Risk-Free Rate

- **Value:** 6% (approximate Indian government bond yield)
- **Usage:** Sharpe Ratio calculation
- **Note:** Could be made configurable for different markets

---

## API Design

### Response Format

**Success Response:**

```json
{
  "success": true,
  "data": { }
}
```

**Error Response:**

```json
{
  "error": "error_code",
  "message": "Human readable description"
}
```

### Endpoints

| Endpoint | Method | Purpose |
|:---------|:------:|:--------|
| `/health` | GET | Health check |
| `/api/v1/funds` | GET | List all tracked funds |
| `/api/v1/funds/:code` | GET | Fund details + latest NAV |
| `/api/v1/funds/:code/analytics` | GET | Pre-computed analytics |
| `/api/v1/funds/rank` | GET | Rank funds by metric |
| `/api/v1/sync/trigger` | POST | Trigger data sync |
| `/api/v1/sync/status` | GET | Pipeline status |

### Input Validation

| Parameter | Validation | Error Response |
|:----------|:-----------|:---------------|
| `window` | Must be `1Y`, `3Y`, `5Y`, `10Y` | 400 Bad Request |
| `sort_by` | Must be valid metric name | 400 Bad Request |
| `limit` | Integer 1-100 | Defaults to 10 |
| `scheme_code` | Valid integer | 400 Bad Request |

---

## Performance

### Database Indexes

```sql
-- Fast fund lookup
CREATE INDEX idx_funds_scheme_code ON funds(scheme_code);

-- NAV range queries
CREATE INDEX idx_nav_history_fund_date ON nav_history(fund_id, date DESC);

-- Analytics lookup
CREATE INDEX idx_fund_analytics_fund_window ON fund_analytics(fund_id, window_id);

-- Pending sync queries
CREATE INDEX idx_sync_state_status ON sync_state(status);
```

### Response Time Target

| Target | How Achieved |
|:-------|:-------------|
| **< 200ms** | Pre-computed analytics stored in database |
| | Efficient indexing for all query patterns |
| | Connection pooling |
| | Minimal memory allocations |

---

## Error Handling

### Error Wrapping

```go
// All errors wrapped with context
return fmt.Errorf("failed to fetch fund %d: %w", schemeCode, err)
```

**Benefits:**
- Full error chain preserved
- Context added at each layer
- Enables `errors.Is/As` for handling

### Retry Strategy

| Aspect | Value |
|:-------|:------|
| **Max Retries** | Configurable (default: 3) |
| **Backoff** | Exponential (delay × retry count) |
| **Tracking** | Retry count stored per fund |

---

## Testing

### Test Layers

| Layer | Type | Description |
|:------|:-----|:------------|
| **Services** | Unit | Individual method testing |
| **Handlers** | Integration | Full HTTP request/response |
| **Rate Limiter** | Unit | Window reset, blocking behavior |

### In-Memory Database

```go
cfg := config.DatabaseConfig{
    Path: ":memory:",  // In-memory SQLite for tests
}
```

**Benefits:**
- Fast test execution
- No cleanup required
- Isolated test state
- No file system dependencies

---

## Tracked Funds

| AMC | Mid Cap | Small Cap |
|:----|:-------:|:---------:|
| **ICICI Prudential** | 120505 | 120586 |
| **HDFC** | 118989 | 130503 |
| **Axis** | 119028 | 125354 |
| **SBI** | 119551 | 125497 |
| **Kotak Mahindra** | 118668 | 125494 |

**Categories:**
- Equity Mid Cap Direct Growth
- Equity Small Cap Direct Growth

---

## Concurrency & Safety

### Thread Safety

| Resource | Protection |
|:---------|:-----------|
| Rate limiter counters | `sync.Mutex` |
| Sync running state | `sync.Mutex` |
| Database writes | SQLite WAL + transactions |

### Context Propagation

```go
// Context passed through all layers
func (s *FundService) GetFund(ctx context.Context, code int64) (*Fund, error)
```

**Benefits:**
- Request cancellation support
- Timeout propagation
- Graceful shutdown handling

---

## Future Improvements

| Improvement | Benefit |
|:------------|:--------|
| **Redis Caching** | API response caching, faster reads |
| **Background Workers** | Dedicated sync worker process |
| **Prometheus Metrics** | Observability and monitoring |
| **API Rate Limiting** | Client-side rate limiting |
| **Webhook Notifications** | Sync completion callbacks |
| **Historical Analytics** | Analytics history for trend analysis |
| **Horizontal Scaling** | PostgreSQL migration for multi-instance |

---

<p align="center">
  <i>Architecture decisions prioritize simplicity, reliability, and maintainability.</i>
</p>
