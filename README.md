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

### API Endpoint

**GET** `/api/chrome/version`

#### Query Parameters

- `platform` (optional): Platform to query. Default: `win64`
  - Valid values: `win`, `win64`, `mac`, `mac_arm64`, `linux`
- `offset` (optional): Version offset to calculate supported version. Overrides `VERSION_OFFSET` environment variable for this request only.
  - Must be a non-negative integer

#### Examples

```bash
# Get latest and supported versions with default offset (from env or 10)
curl http://localhost:8080/api/chrome/version

# Get versions for macOS
curl http://localhost:8080/api/chrome/version?platform=mac

# Override offset for this request only
curl http://localhost:8080/api/chrome/version?offset=5

# Combine parameters
curl http://localhost:8080/api/chrome/version?platform=mac&offset=15
```

#### Response

```json
{
  "latest": "143.0.7499.41",
  "latest_accepted": "133.0.6943.143",
  "channel": "stable",
  "platform": "win64"
}
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

## API Reference

### Google Version History API

This service uses the official Google Chrome Version History API:
- Base URL: `https://versionhistory.googleapis.com/v1`
- Endpoint: `/chrome/platforms/{platform}/channels/stable/versions`
- Documentation: [Chrome Version History API](https://developer.chrome.com/docs/versionhistory/reference)

## Error Handling

The service returns appropriate HTTP status codes and error messages:

- `400 Bad Request`: Invalid platform or offset parameter
- `404 Not Found`: No versions found for the specified platform
- `500 Internal Server Error`: API communication errors

## License

MIT
