package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/versionhistory/v1"
)

// VersionResponse represents the main API response
type VersionResponse struct {
	Latest         string `json:"latest"`
	LatestAccepted string `json:"latest_accepted"`
	Channel        string `json:"channel"`
	Platform       string `json:"platform"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error string `json:"error"`
}

func main() {
	http.HandleFunc("/api/chrome/version", getChromeVersions)

	port := ":8080"
	log.Printf("Server started on http://localhost%s", port)
	log.Printf("Endpoint: GET /api/chrome/version?platform=win64&offset=10")
	log.Printf("VERSION_OFFSET=%s (default: 10)", getVersionOffset())
	log.Printf("Use ?offset=N to override VERSION_OFFSET for a single request")
	log.Fatal(http.ListenAndServe(port, nil))
}

// getChromeVersions handles the main request
func getChromeVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get parameters
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "win64"
	}

	// Validate platform
	if !isValidPlatform(platform) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "Invalid platform. Use: win, win64, mac, mac_arm64, linux",
		})
		return
	}

	// Get offset: priority to query parameter, then environment variable
	offset := getOffsetFromRequest(r)
	if offset < 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "Offset must be a number >= 0",
		})
		return
	}

	// Create Google API client
	ctx := context.Background()
	service, err := versionhistory.NewService(ctx, option.WithoutAuthentication())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: fmt.Sprintf("Error creating service: %v", err),
		})
		return
	}

	// Call Google API to get all versions
	log.Printf("Calling Google API for platform=%s, offset=%d", platform, offset)
	parent := fmt.Sprintf("chrome/platforms/%s/channels/stable", platform)
	call := service.Platforms.Channels.Versions.List(parent)
	call = call.PageSize(1000) // Get many versions to be sure
	call = call.OrderBy("version desc")

	response, err := call.Do()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: fmt.Sprintf("Error calling API: %v", err),
		})
		return
	}

	if len(response.Versions) == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "No versions found",
		})
		return
	}

	// 1. The LATEST is the first version (highest major)
	latest := response.Versions[0].Version
	latestMajor := extractMajor(latest)
	log.Printf("Latest version: %s (major: %d)", latest, latestMajor)

	// 2. Calculate supported major: latest_major - offset
	supportedMajor := latestMajor - offset
	log.Printf("Supported major: %d (latest %d - offset %d)", supportedMajor, latestMajor, offset)

	// 3. Find the first version with the supported major
	latestAccepted := findFirstVersionWithMajor(response.Versions, supportedMajor)
	if latestAccepted == "" {
		log.Printf("No version found for major %d", supportedMajor)
		latestAccepted = fmt.Sprintf("%d.0.0.0", supportedMajor)
	} else {
		log.Printf("Latest accepted: %s", latestAccepted)
	}

	// Return result
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(VersionResponse{
		Latest:         latest,
		LatestAccepted: latestAccepted,
		Channel:        "stable",
		Platform:       platform,
	})
}

// extractMajor extracts the major number from a version (e.g.: "143.0.7499.41" -> 143)
func extractMajor(version string) int {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0
	}
	major, _ := strconv.Atoi(parts[0])
	return major
}

// findFirstVersionWithMajor finds the first version with a specific major
func findFirstVersionWithMajor(versions []*versionhistory.Version, targetMajor int) string {
	for _, v := range versions {
		if extractMajor(v.Version) == targetMajor {
			return v.Version
		}
	}
	return ""
}

// getVersionOffset reads VERSION_OFFSET from env, default "10"
func getVersionOffset() string {
	offset := os.Getenv("VERSION_OFFSET")
	if offset == "" {
		return "10"
	}
	return offset
}

// getVersionOffsetInt reads VERSION_OFFSET as int
func getVersionOffsetInt() int {
	offsetStr := getVersionOffset()
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		return 10
	}
	return offset
}

// isValidPlatform checks if the platform is valid
func isValidPlatform(platform string) bool {
	valid := map[string]bool{
		"win":       true,
		"win64":     true,
		"mac":       true,
		"mac_arm64": true,
		"linux":     true,
	}
	return valid[platform]
}

// getOffsetFromRequest reads the offset from query parameter or environment variable
// Priority: 1. Query parameter "offset", 2. Environment variable VERSION_OFFSET, 3. Default 10
func getOffsetFromRequest(r *http.Request) int {
	// 1. Check if there's an offset parameter in the query
	offsetParam := r.URL.Query().Get("offset")
	if offsetParam != "" {
		offset, err := strconv.Atoi(offsetParam)
		if err != nil {
			return -1 // Invalid value
		}
		return offset
	}

	// 2. Use environment variable or default
	return getVersionOffsetInt()
}
