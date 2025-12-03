# Chrome Version Service

A simple Go service that exposes a REST API to retrieve the latest stable Chrome version and calculate the supported version based on a configurable offset.

## Features

- 🚀 Get the latest stable Chrome version from Google's Version History API
- 🔢 Calculate supported version based on major version offset
- 🌍 Support for multiple platforms (win, win64, mac, mac_arm64, linux)
- ⚙️ Configurable offset via environment variable or query parameter
- 📊 Returns actual released versions (not synthetic ones)

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

1. **Fetch versions**: Calls Google's Version History API to get all stable Chrome versions for the specified platform
2. **Identify latest**: The first version in the response (highest major version)
3. **Calculate supported major**: `latest_major - offset`
4. **Find latest_accepted**: Searches through the version list to find the first actual version with the calculated major

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
