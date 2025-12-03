# Chrome Version Service

A simple Go service that exposes a REST API to retrieve the latest stable Chrome version and calculate the supported version based on a configurable offset.

## Features

- 🚀 Get the latest stable Chrome version from Google's Version History API
- 🔢 Calculate supported version based on major version offset
- 🌍 Support for multiple platforms (win, win64, mac, mac_arm64, linux)
- ⚙️ Configurable offset via environment variable or query parameter
- 📊 Returns actual released versions (not synthetic ones)
- ⚡ In-memory cache with 24-hour TTL to minimize API calls
- 🔄 Automatic cache cleanup every hour

## Installation

```bash
# Clone or download the project
cd "c:\repositories\check wks"

# Initialize Go module (if not already done)
go mod init chrome-version-service
go mod tidy

# Build the service
go build main.go
```

## Usage

### Start the service

```bash
# With default offset (10)
go run main.go

# With custom offset via environment variable
# PowerShell
$env:VERSION_OFFSET="5"
go run main.go

# Linux/macOS
export VERSION_OFFSET=5
go run main.go
```

## API Endpoints

### 1. GET `/api/chrome/version`

Retrieve the latest stable Chrome version and calculate the supported version based on offset.

#### Request

**URL**: `http://localhost:8080/api/chrome/version`

**Method**: `GET`

**Query Parameters**:

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `platform` | string | No | `win64` | Target platform. Valid: `win`, `win64`, `mac`, `mac_arm64`, `linux` |
| `offset` | integer | No | `VERSION_OFFSET` env or `10` | Major version offset. Must be >= 0. Overrides environment variable. |

#### Response

**Success (200 OK)**:

```json
{
  "latest": "143.0.7499.41",
  "latest_accepted": "133.0.6943.143",
  "channel": "stable",
  "platform": "win64"
}
```

**Fields**:
- `latest`: Latest stable Chrome version available
- `latest_accepted`: Calculated supported version (latest major - offset)
- `channel`: Always `"stable"`
- `platform`: Platform queried

**Error Responses**:

```json
// 400 Bad Request - Invalid platform
{
  "error": "Invalid platform. Use: win, win64, mac, mac_arm64, linux"
}

// 400 Bad Request - Invalid offset
{
  "error": "Offset must be a number >= 0"
}

// 404 Not Found - No versions found
{
  "error": "No versions found"
}

// 500 Internal Server Error - API error
{
  "error": "Error calling API: connection timeout"
}
```

#### Examples

```bash
# Basic request with defaults
curl http://localhost:8080/api/chrome/version

# Specific platform
curl http://localhost:8080/api/chrome/version?platform=mac

# Custom offset
curl http://localhost:8080/api/chrome/version?offset=5

# Combined parameters
curl http://localhost:8080/api/chrome/version?platform=linux&offset=15

# Pretty print with jq
curl -s http://localhost:8080/api/chrome/version | jq

# PowerShell
Invoke-RestMethod -Uri "http://localhost:8080/api/chrome/version?platform=win64&offset=10"
```

---

### 2. GET `/health`

Health check endpoint for monitoring service status, Google API connectivity, and cache performance.

#### Request

**URL**: `http://localhost:8080/health`

**Method**: `GET`

**Query Parameters**: None

#### Response

**Healthy (200 OK)**:

```json
{
  "status": "healthy",
  "timestamp": "2025-12-03T14:15:00Z",
  "uptime": "0d 2h 15m 30s",
  "google_api_status": "healthy",
  "cache_stats": {
    "total_entries": 5,
    "active_entries": 5,
    "expired_entries": 0,
    "hit_rate": "75.50%"
  },
  "last_api_call": "2025-12-03T14:10:00Z"
}
```

**Degraded (503 Service Unavailable)**:

```json
{
  "status": "degraded",
  "timestamp": "2025-12-03T14:15:00Z",
  "uptime": "0d 2h 15m 30s",
  "google_api_status": "unhealthy",
  "cache_stats": {
    "total_entries": 3,
    "active_entries": 3,
    "expired_entries": 0,
    "hit_rate": "60.00%"
  },
  "last_api_call": "2025-12-03T14:12:00Z",
  "last_api_error": "Error calling API: connection timeout"
}
```

**Fields**:
- `status`: Overall service status (`healthy` or `degraded`)
- `timestamp`: Current server time (RFC3339 format)
- `uptime`: Service uptime (days, hours, minutes, seconds)
- `google_api_status`: Google API connectivity (`healthy` or `unhealthy`)
- `cache_stats`: Cache performance metrics
  - `total_entries`: Total cached entries
  - `active_entries`: Non-expired entries
  - `expired_entries`: Expired entries (pending cleanup)
  - `hit_rate`: Cache hit percentage
- `last_api_call`: Last Google API call timestamp (omitted if never called)
- `last_api_error`: Last error message (only if error occurred)

#### Examples

```bash
# Check health
curl http://localhost:8080/health

# Monitor continuously (Linux/macOS)
watch -n 5 'curl -s http://localhost:8080/health | jq'

# Monitor continuously (PowerShell)
while($true) { 
  Invoke-RestMethod http://localhost:8080/health | ConvertTo-Json -Depth 10
  Start-Sleep 5 
}

# Check if healthy (exit code 0 if healthy)
curl -f http://localhost:8080/health > /dev/null 2>&1 && echo "Healthy" || echo "Degraded"
```

## How It Works

1. **Check cache**: First checks if a valid cached response exists for the platform/offset combination
2. **Fetch versions**: If cache miss, calls Google's Version History API to get all stable Chrome versions for the specified platform
3. **Identify latest**: The first version in the response (highest major version)
4. **Calculate supported major**: `latest_major - offset`
5. **Find latest_accepted**: Searches through the version list to find the first actual version with the calculated major
6. **Cache result**: Stores the result in memory with a 24-hour expiration time

### Example Calculation

- Latest version: `143.0.7499.41` (major: 143)
- Offset: 10
- Supported major: 143 - 10 = 133
- Latest accepted: First version found with major 133 → `133.0.6943.143`

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `VERSION_OFFSET` | Number of major versions to subtract from latest to calculate supported version | `10` |

**Note**: The `offset` query parameter takes precedence over the `VERSION_OFFSET` environment variable for individual requests.

## Cache Behavior

The service implements an in-memory cache to reduce unnecessary calls to Google's API:

- **Cache Key**: Combination of `platform` and `offset` (e.g., `win64:10`, `mac:5`)
- **TTL**: 24 hours from the time of caching
- **Invalidation**: Automatic cleanup runs every hour to remove expired entries
- **Behavior**:
  - First request for a platform/offset combination → **Cache MISS** → Calls Google API → Stores result
  - Subsequent requests within 24 hours → **Cache HIT** → Returns cached result immediately
  - Requests after 24 hours → **Cache MISS** → Calls Google API again → Updates cache

### Cache Examples

```bash
# First request - Cache MISS
curl http://localhost:8080/api/chrome/version?platform=win64&offset=10
# Logs: "Cache MISS for platform=win64, offset=10"
# Logs: "Calling Google API for platform=win64, offset=10"
# Logs: "Cached result for platform=win64, offset=10 (expires in 24h)"

# Second request within 24h - Cache HIT
curl http://localhost:8080/api/chrome/version?platform=win64&offset=10
# Logs: "Cache HIT for platform=win64, offset=10"
# No Google API call made

# Different offset - Cache MISS (different key)
curl http://localhost:8080/api/chrome/version?platform=win64&offset=5
# Logs: "Cache MISS for platform=win64, offset=5"
# Logs: "Calling Google API for platform=win64, offset=5"
```

## Use Cases

### 1. CI/CD Pipeline Integration

Check if your application supports the latest Chrome version:

```bash
#!/bin/bash
# Get supported version with offset 5
SUPPORTED=$(curl -s "http://localhost:8080/api/chrome/version?offset=5" | jq -r '.latest_accepted')
echo "Testing with Chrome $SUPPORTED"

# Use in Docker/Selenium
docker run -e CHROME_VERSION=$SUPPORTED selenium/standalone-chrome:$SUPPORTED
```

### 2. Monitoring Dashboard

Integrate with Prometheus/Grafana:

```bash
# Health check for alerting
curl -f http://localhost:8080/health || alert "Chrome Version Service Down"

# Get cache hit rate
HIT_RATE=$(curl -s http://localhost:8080/health | jq -r '.cache_stats.hit_rate')
echo "cache_hit_rate $HIT_RATE"
```

### 3. Automated Version Updates

Update configuration files automatically:

```python
import requests
import json

# Get latest versions
response = requests.get('http://localhost:8080/api/chrome/version?platform=linux&offset=10')
data = response.json()

print(f"Latest: {data['latest']}")
print(f"Supported: {data['latest_accepted']}")

# Update config file
config = {
    'chrome_latest': data['latest'],
    'chrome_supported': data['latest_accepted']
}

with open('config.json', 'w') as f:
    json.dump(config, f, indent=2)
```

### 4. Multi-Platform Testing

Get versions for all platforms:

```bash
#!/bin/bash
for platform in win win64 mac mac_arm64 linux; do
  echo "Platform: $platform"
  curl -s "http://localhost:8080/api/chrome/version?platform=$platform&offset=10" | jq
  echo "---"
done
```

### 5. Kubernetes Liveness/Readiness Probes

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: chrome-version-proxy
spec:
  containers:
  - name: app
    image: chrome-version-proxy:latest
    ports:
    - containerPort: 8080
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 10
      periodSeconds: 30
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
```

## HTTP Status Codes

| Code | Status | Description |
|------|--------|-------------|
| 200 | OK | Request successful |
| 400 | Bad Request | Invalid parameters (platform, offset) |
| 404 | Not Found | No versions found for platform |
| 500 | Internal Server Error | Google API error or service error |
| 503 | Service Unavailable | Health check failed (degraded state) |

## API Reference

### Google Version History API

This service uses the official Google Chrome Version History API:
- **Base URL**: `https://versionhistory.googleapis.com/v1`
- **Endpoint**: `/chrome/platforms/{platform}/channels/stable/versions`
- **Documentation**: [Chrome Version History API](https://developer.chrome.com/docs/versionhistory/reference)
- **Authentication**: None required (public API)

## Performance

- **First request**: ~500-1000ms (Google API call)
- **Cached request**: <5ms (in-memory cache)
- **Cache TTL**: 24 hours
- **Cleanup interval**: 1 hour
- **Concurrent requests**: Supported (thread-safe cache)

## License

MIT
