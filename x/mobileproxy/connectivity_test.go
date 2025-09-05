// Copyright 2024 The Outline Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mobileproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectivityResult represents the result of a single connectivity test.
type ConnectivityResult struct {
	// Config is the configuration that was tested (e.g., access key)
	Config string `json:"config"`
	// Success indicates whether the test passed
	Success bool `json:"success"`
	// DurationMs is the time taken for the test in milliseconds
	DurationMs int64 `json:"durationMs"`
	// Error contains detailed error information if the test failed
	Error *ConnectivityError `json:"error,omitempty"`
	// TestURL is the URL that was tested
	TestURL string `json:"testURL,omitempty"`
}

// ToJSON returns a JSON string representation of the ConnectivityResult.
func (r *ConnectivityResult) ToJSON() (string, error) {
	return marshalToJSON(r)
}

// ConnectivityCallback defines the interface for receiving connectivity test progress updates.
// This interface is designed to work with Go Mobile bindings.
type ConnectivityCallback interface {
	// OnProgress is called when a single test completes (success or failure)
	OnProgress(result *ConnectivityResult)
	// OnCompleted is called when all tests are finished
	OnCompleted(summary *BatchTestSummary)
}

// BatchTestSummary provides an overview of batch connectivity testing results.
type BatchTestSummary struct {
	// TotalTests is the number of configurations tested
	TotalTests int `json:"totalTests"`
	// SuccessfulTests is the number of tests that passed
	SuccessfulTests int `json:"successfulTests"`
	// FailedTests is the number of tests that failed
	FailedTests int `json:"failedTests"`
	// BestConfig is the configuration with the lowest latency (if any succeeded)
	BestConfig string `json:"bestConfig,omitempty"`
	// BestLatencyMs is the latency of the best configuration in milliseconds
	BestLatencyMs int64 `json:"bestLatencyMs,omitempty"`
	// TotalDurationMs is the total time taken for all tests in milliseconds
	TotalDurationMs int64 `json:"totalDurationMs"`
}

// ToJSON returns a JSON string representation of the BatchTestSummary.
func (s *BatchTestSummary) ToJSON() (string, error) {
	return marshalToJSON(s)
}

// BatchTestController allows control over a running batch connectivity test.
type BatchTestController struct {
	cancel    context.CancelFunc
	done      chan struct{}
	cancelled atomic.Bool
}

// Cancel stops the batch connectivity test.
func (c *BatchTestController) Cancel() {
	if c.cancelled.CompareAndSwap(false, true) {
		c.cancel()
	}
}

// Wait blocks until the batch test completes or is cancelled.
func (c *BatchTestController) Wait() {
	<-c.done
}

// IsCancelled returns true if the test has been cancelled.
func (c *BatchTestController) IsCancelled() bool {
	return c.cancelled.Load()
}

// CheckHTTPConnectivity tests HTTP connectivity using the given dialer.
// If testURL is empty, the test is skipped and success is returned.
// Returns a ConnectivityResult with detailed information about the test.
func CheckHTTPConnectivity(dialer *StreamDialer, testURL string) *ConnectivityResult {
	config := "unknown"
	if dialer != nil {
		config = "provided"
	}
	
	result := &ConnectivityResult{
		Config:  config,
		TestURL: testURL,
	}
	
	// Skip test if no URL provided
	if testURL == "" {
		result.Success = true
		result.DurationMs = 0
		return result
	}
	
	// Validate input parameters
	if dialer == nil {
		result.Error = NewConnectivityError(IllegalConfig, "dialer cannot be nil")
		return result
	}
	
	start := time.Now()
	err := performHTTPConnectivityTest(dialer, testURL)
	result.DurationMs = time.Since(start).Milliseconds()
	
	if err != nil {
		result.Error = ToConnectivityError(err)
		result.Success = false
	} else {
		result.Success = true
	}
	
	return result
}

// CheckMultipleConfigs tests connectivity for multiple configurations concurrently.
// Each configuration should be a valid transport config string (e.g., Shadowsocks access key).
// testURL specifies the URL to test (if empty, tests are skipped).
// callback receives progress updates for each completed test.
// Returns a BatchTestController that can be used to cancel the operation.
func CheckMultipleConfigs(configs *StringList, testURL string, callback ConnectivityCallback) *BatchTestController {
	ctx, cancel := context.WithCancel(context.Background())
	controller := &BatchTestController{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	
	go func() {
		defer close(controller.done)
		runBatchConnectivityTest(ctx, configs, testURL, callback, controller)
	}()
	
	return controller
}

// performHTTPConnectivityTest performs the actual HTTP connectivity test.
func performHTTPConnectivityTest(dialer *StreamDialer, testURL string) error {
	// Parse and validate the test URL
	parsedURL, err := url.Parse(testURL)
	if err != nil {
		return NewConnectivityError(IllegalConfig, fmt.Sprintf("invalid test URL: %v", err))
	}
	
	// Set default port if not specified (validation only)
	if parsedURL.Port() == "" {
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return NewConnectivityError(IllegalConfig, "test URL must use http or https scheme")
		}
	}
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Create HTTP client with custom transport using our dialer
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialStream(ctx, addr)
			},
		},
		Timeout: 10 * time.Second,
	}
	
	// Create HTTP HEAD request
	req, err := http.NewRequestWithContext(ctx, "HEAD", testURL, nil)
	if err != nil {
		return NewConnectivityError(InternalError, fmt.Sprintf("failed to create request: %v", err))
	}
	
	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		// Map specific error types to connectivity error codes
		if ctx.Err() == context.DeadlineExceeded {
			return NewConnectivityError(TimeoutError, "connectivity test timed out")
		}
		return NewConnectivityError(ProxyServerUnreachable, fmt.Sprintf("failed to connect: %v", err))
	}
	defer resp.Body.Close()
	
	// Check HTTP status code
	if resp.StatusCode >= 400 {
		details := ErrorDetails{
			"statusCode": resp.StatusCode,
			"status":     resp.Status,
		}
		return NewConnectivityErrorWithDetails(
			ProxyServerReadFailed,
			fmt.Sprintf("HTTP test returned status: %d", resp.StatusCode),
			details,
		)
	}
	
	return nil
}

// runBatchConnectivityTest performs batch connectivity testing.
func runBatchConnectivityTest(ctx context.Context, configs *StringList, testURL string, callback ConnectivityCallback, controller *BatchTestController) {
	if configs == nil || len(configs.list) == 0 {
		summary := &BatchTestSummary{
			TotalTests:      0,
			SuccessfulTests: 0,
			FailedTests:     0,
		}
		if callback != nil {
			callback.OnCompleted(summary)
		}
		return
	}
	
	start := time.Now()
	var wg sync.WaitGroup
	results := make(chan *ConnectivityResult, len(configs.list))
	
	// Test each configuration concurrently
	for _, config := range configs.list {
		if controller.IsCancelled() {
			break
		}
		
		wg.Add(1)
		go func(cfg string) {
			defer wg.Done()
			
			select {
			case <-ctx.Done():
				return
			default:
			}
			
			result := testSingleConfig(ctx, cfg, testURL)
			
			select {
			case results <- result:
				if callback != nil {
					callback.OnProgress(result)
				}
			case <-ctx.Done():
				return
			}
		}(config)
	}
	
	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()
	
	// Collect results and find the best configuration
	summary := &BatchTestSummary{
		TotalTests:      len(configs.list),
		TotalDurationMs: 0,
	}
	
	var bestLatency int64 = -1
	
	for result := range results {
		if controller.IsCancelled() {
			break
		}
		
		if result.Success {
			summary.SuccessfulTests++
			// Track the best (lowest latency) successful configuration
			if bestLatency == -1 || result.DurationMs < bestLatency {
				bestLatency = result.DurationMs
				summary.BestConfig = result.Config
				summary.BestLatencyMs = result.DurationMs
			}
		} else {
			summary.FailedTests++
		}
	}
	
	summary.TotalDurationMs = time.Since(start).Milliseconds()
	
	if callback != nil {
		callback.OnCompleted(summary)
	}
}

// testSingleConfig tests a single configuration string.
func testSingleConfig(ctx context.Context, config string, testURL string) *ConnectivityResult {
	result := &ConnectivityResult{
		Config:  config,
		TestURL: testURL,
	}
	
	// Skip test if no URL provided
	if testURL == "" {
		result.Success = true
		result.DurationMs = 0
		return result
	}
	
	start := time.Now()
	
	// Create dialer from config
	dialer, err := NewStreamDialerFromConfig(config)
	if err != nil {
		result.Error = NewConnectivityError(IllegalConfig, fmt.Sprintf("invalid config: %v", err))
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	
	// Perform connectivity test with context
	err = performHTTPConnectivityTestWithContext(ctx, dialer, testURL)
	result.DurationMs = time.Since(start).Milliseconds()
	
	if err != nil {
		result.Error = ToConnectivityError(err)
		result.Success = false
	} else {
		result.Success = true
	}
	
	return result
}

// performHTTPConnectivityTestWithContext performs HTTP connectivity test with cancellation support.
func performHTTPConnectivityTestWithContext(ctx context.Context, dialer *StreamDialer, testURL string) error {
	// Parse and validate the test URL
	_, err := url.Parse(testURL)
	if err != nil {
		return NewConnectivityError(IllegalConfig, fmt.Sprintf("invalid test URL: %v", err))
	}
	
	// Create HTTP client with custom transport using our dialer
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialStream(dialCtx, addr)
			},
		},
		Timeout: 10 * time.Second,
	}
	
	// Create HTTP HEAD request with context
	req, err := http.NewRequestWithContext(ctx, "HEAD", testURL, nil)
	if err != nil {
		return NewConnectivityError(InternalError, fmt.Sprintf("failed to create request: %v", err))
	}
	
	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return NewConnectivityError(TimeoutError, "connectivity test was cancelled or timed out")
		}
		return NewConnectivityError(ProxyServerUnreachable, fmt.Sprintf("failed to connect: %v", err))
	}
	defer resp.Body.Close()
	
	// Check HTTP status code
	if resp.StatusCode >= 400 {
		details := ErrorDetails{
			"statusCode": resp.StatusCode,
			"status":     resp.Status,
		}
		return NewConnectivityErrorWithDetails(
			ProxyServerReadFailed,
			fmt.Sprintf("HTTP test returned status: %d", resp.StatusCode),
			details,
		)
	}
	
	return nil
}

// Helper function to marshal structs to JSON strings
func marshalToJSON(v interface{}) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}