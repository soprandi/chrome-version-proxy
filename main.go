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
	"sync"
	"time"

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

// HealthResponse represents the health check response
type HealthResponse struct {
	Status           string            `json:"status"`
	Timestamp        string            `json:"timestamp"`
	Uptime           string            `json:"uptime"`
	GoogleAPIStatus  string            `json:"google_api_status"`
	CacheStats       CacheStats        `json:"cache_stats"`
	LastAPICall      string            `json:"last_api_call,omitempty"`
	LastAPIError     string            `json:"last_api_error,omitempty"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalEntries   int    `json:"total_entries"`
	ActiveEntries  int    `json:"active_entries"`
	ExpiredEntries int    `json:"expired_entries"`
	HitRate        string `json:"hit_rate,omitempty"`
}

// CacheEntry represents a cached response with expiration time
type CacheEntry struct {
	Response  VersionResponse
	ExpiresAt time.Time
}

// Cache stores version responses by platform and offset
type Cache struct {
	mu            sync.RWMutex
	entries       map[string]CacheEntry
	hits          int64
	misses        int64
	lastAPICall   time.Time
	lastAPIError  string
	apiHealthy    bool
}

// Global cache instance
var (
	cache     = &Cache{
		entries:    make(map[string]CacheEntry),
		apiHealthy: true,
	}
	startTime = time.Now()
)

const cacheTTL = 24 * time.Hour

func main() {
	http.HandleFunc("/api/chrome/version", getChromeVersions)
	http.HandleFunc("/health", healthCheck)

	// Start cache cleanup goroutine
	go cleanupExpiredCache()

	port := ":8080"
	log.Printf("Server started on http://localhost%s", port)
	log.Printf("Endpoints:")
	log.Printf("  - GET /api/chrome/version?platform=win64&offset=10")
	log.Printf("  - GET /health")
	log.Printf("VERSION_OFFSET=%s (default: 10)", getVersionOffset())
	log.Printf("Use ?offset=N to override VERSION_OFFSET for a single request")
	log.Printf("Cache TTL: 24 hours")
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

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%d", platform, offset)
	if cachedResponse, found := cache.Get(cacheKey); found {
		cache.recordHit()
		log.Printf("Cache HIT for platform=%s, offset=%d", platform, offset)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(cachedResponse)
		return
	}

	cache.recordMiss()
	log.Printf("Cache MISS for platform=%s, offset=%d", platform, offset)

	// Create Google API client
	ctx := context.Background()
	service, err := versionhistory.NewService(ctx, option.WithoutAuthentication())
	if err != nil {
		cache.recordAPIError(err)
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

	apiResponse, err := call.Do()
	if err != nil {
		cache.recordAPIError(err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: fmt.Sprintf("Error calling API: %v", err),
		})
		return
	}

	// Record successful API call
	cache.recordAPISuccess()

	if len(apiResponse.Versions) == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "No versions found",
		})
		return
	}

	// 1. The LATEST is the first version (highest major)
	latest := apiResponse.Versions[0].Version
	latestMajor := extractMajor(latest)
	log.Printf("Latest version: %s (major: %d)", latest, latestMajor)

	// 2. Calculate supported major: latest_major - offset
	supportedMajor := latestMajor - offset
	log.Printf("Supported major: %d (latest %d - offset %d)", supportedMajor, latestMajor, offset)

	// 3. Find the first version with the supported major
	latestAccepted := findFirstVersionWithMajor(apiResponse.Versions, supportedMajor)
	if latestAccepted == "" {
		log.Printf("No version found for major %d", supportedMajor)
		latestAccepted = fmt.Sprintf("%d.0.0.0", supportedMajor)
	} else {
		log.Printf("Latest accepted: %s", latestAccepted)
	}

	// Build response
	response := VersionResponse{
		Latest:         latest,
		LatestAccepted: latestAccepted,
		Channel:        "stable",
		Platform:       platform,
	}

	// Store in cache
	cache.Set(cacheKey, response)
	log.Printf("Cached result for platform=%s, offset=%d (expires in 24h)", platform, offset)

	// Return result
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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

// Get retrieves a cached entry if it exists and hasn't expired
func (c *Cache) Get(key string) (VersionResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[key]
	if !found {
		return VersionResponse{}, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return VersionResponse{}, false
	}

	return entry.Response, true
}

// Set stores a response in the cache with 24h expiration
func (c *Cache) Set(key string, response VersionResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(cacheTTL),
	}
}

// cleanupExpiredCache removes expired entries every hour
func cleanupExpiredCache() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cache.mu.Lock()
		now := time.Now()
		count := 0
		for key, entry := range cache.entries {
			if now.After(entry.ExpiresAt) {
				delete(cache.entries, key)
				count++
			}
		}
		if count > 0 {
			log.Printf("Cache cleanup: removed %d expired entries", count)
		}
		cache.mu.Unlock()
	}
}

// healthCheck handles the health check endpoint
func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Calculate uptime
	uptime := time.Since(startTime)
	uptimeStr := fmt.Sprintf("%dd %dh %dm %ds",
		int(uptime.Hours())/24,
		int(uptime.Hours())%24,
		int(uptime.Minutes())%60,
		int(uptime.Seconds())%60)

	// Get cache stats
	stats := cache.getStats()

	// Determine overall status
	status := "healthy"
	if !cache.isAPIHealthy() {
		status = "degraded"
	}

	// Build response
	response := HealthResponse{
		Status:          status,
		Timestamp:       time.Now().Format(time.RFC3339),
		Uptime:          uptimeStr,
		GoogleAPIStatus: cache.getAPIStatus(),
		CacheStats:      stats,
	}

	// Add last API call time if available
	if !cache.lastAPICall.IsZero() {
		response.LastAPICall = cache.lastAPICall.Format(time.RFC3339)
	}

	// Add last error if present
	if cache.lastAPIError != "" {
		response.LastAPIError = cache.lastAPIError
	}

	// Return appropriate status code
	if status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// recordHit increments cache hit counter
func (c *Cache) recordHit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
}

// recordMiss increments cache miss counter
func (c *Cache) recordMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.misses++
}

// recordAPISuccess marks the API as healthy
func (c *Cache) recordAPISuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAPICall = time.Now()
	c.apiHealthy = true
	c.lastAPIError = ""
}

// recordAPIError marks the API as unhealthy
func (c *Cache) recordAPIError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAPICall = time.Now()
	c.apiHealthy = false
	c.lastAPIError = err.Error()
}

// isAPIHealthy returns the current API health status
func (c *Cache) isAPIHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiHealthy
}

// getAPIStatus returns a human-readable API status
func (c *Cache) getAPIStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.apiHealthy {
		return "healthy"
	}
	return "unhealthy"
}

// getStats returns cache statistics
func (c *Cache) getStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	active := 0
	expired := 0

	for _, entry := range c.entries {
		if now.Before(entry.ExpiresAt) {
			active++
		} else {
			expired++
		}
	}

	stats := CacheStats{
		TotalEntries:   len(c.entries),
		ActiveEntries:  active,
		ExpiredEntries: expired,
	}

	// Calculate hit rate
	total := c.hits + c.misses
	if total > 0 {
		hitRate := float64(c.hits) / float64(total) * 100
		stats.HitRate = fmt.Sprintf("%.2f%%", hitRate)
	}

	return stats
}
