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

package httpproxy

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/transport"
)

// TrafficStats holds statistics about network traffic and connection durations.
type TrafficStats struct {
	// Total bytes uploaded (sent to server)
	uploadBytes int64
	// Total bytes downloaded (received from server)
	downloadBytes int64
	// Total connection time in nanoseconds
	totalConnectionTime int64
	// Number of active connections
	activeConnections int64
	// Total number of connections established
	totalConnections int64
	// Time when first connection was established
	firstConnectionTime int64
	// Mutex for thread-safe operations on non-atomic fields
	mu sync.RWMutex
	// Map of active connections with their start times
	activeConns map[*monitoredConn]time.Time
}

// NewTrafficStats creates a new TrafficStats instance.
func NewTrafficStats() *TrafficStats {
	return &TrafficStats{
		activeConns: make(map[*monitoredConn]time.Time),
	}
}

// GetUploadBytes returns the total number of bytes uploaded.
func (ts *TrafficStats) GetUploadBytes() int64 {
	return atomic.LoadInt64(&ts.uploadBytes)
}

// GetDownloadBytes returns the total number of bytes downloaded.
func (ts *TrafficStats) GetDownloadBytes() int64 {
	return atomic.LoadInt64(&ts.downloadBytes)
}

// GetTotalConnectionTime returns the total connection time in milliseconds.
func (ts *TrafficStats) GetTotalConnectionTime() int64 {
	return atomic.LoadInt64(&ts.totalConnectionTime) / int64(time.Millisecond)
}

// GetActiveConnections returns the number of currently active connections.
func (ts *TrafficStats) GetActiveConnections() int64 {
	return atomic.LoadInt64(&ts.activeConnections)
}

// GetTotalConnections returns the total number of connections established.
func (ts *TrafficStats) GetTotalConnections() int64 {
	return atomic.LoadInt64(&ts.totalConnections)
}

// GetCurrentSessionDuration returns the duration of the current session in milliseconds.
// This is the time since the first connection was established.
func (ts *TrafficStats) GetCurrentSessionDuration() int64 {
	firstTime := atomic.LoadInt64(&ts.firstConnectionTime)
	if firstTime == 0 {
		return 0
	}
	return time.Since(time.Unix(0, firstTime)).Milliseconds()
}

// Reset clears all statistics.
func (ts *TrafficStats) Reset() {
	atomic.StoreInt64(&ts.uploadBytes, 0)
	atomic.StoreInt64(&ts.downloadBytes, 0)
	atomic.StoreInt64(&ts.totalConnectionTime, 0)
	atomic.StoreInt64(&ts.activeConnections, 0)
	atomic.StoreInt64(&ts.totalConnections, 0)
	atomic.StoreInt64(&ts.firstConnectionTime, 0)
	
	ts.mu.Lock()
	ts.activeConns = make(map[*monitoredConn]time.Time)
	ts.mu.Unlock()
}

// addUpload atomically adds to the upload byte count.
func (ts *TrafficStats) addUpload(bytes int64) {
	atomic.AddInt64(&ts.uploadBytes, bytes)
}

// addDownload atomically adds to the download byte count.
func (ts *TrafficStats) addDownload(bytes int64) {
	atomic.AddInt64(&ts.downloadBytes, bytes)
}

// connectionStarted is called when a new connection is established.
func (ts *TrafficStats) connectionStarted(conn *monitoredConn) {
	now := time.Now()
	
	// Set first connection time if this is the first connection
	atomic.CompareAndSwapInt64(&ts.firstConnectionTime, 0, now.UnixNano())
	
	atomic.AddInt64(&ts.activeConnections, 1)
	atomic.AddInt64(&ts.totalConnections, 1)
	
	ts.mu.Lock()
	ts.activeConns[conn] = now
	ts.mu.Unlock()
}

// connectionEnded is called when a connection is closed.
func (ts *TrafficStats) connectionEnded(conn *monitoredConn) {
	ts.mu.Lock()
	startTime, exists := ts.activeConns[conn]
	if exists {
		delete(ts.activeConns, conn)
		duration := time.Since(startTime)
		atomic.AddInt64(&ts.totalConnectionTime, int64(duration))
	}
	ts.mu.Unlock()
	
	if exists {
		atomic.AddInt64(&ts.activeConnections, -1)
	}
}

// monitoredConn wraps a transport.StreamConn to track traffic statistics.
type monitoredConn struct {
	transport.StreamConn
	stats *TrafficStats
}

// NewMonitoredConn creates a monitored connection that tracks traffic statistics.
func NewMonitoredConn(conn transport.StreamConn, stats *TrafficStats) transport.StreamConn {
	mc := &monitoredConn{
		StreamConn: conn,
		stats:      stats,
	}
	stats.connectionStarted(mc)
	return mc
}

// Read implements the io.Reader interface and tracks downloaded bytes.
func (mc *monitoredConn) Read(b []byte) (n int, err error) {
	n, err = mc.StreamConn.Read(b)
	if n > 0 {
		mc.stats.addDownload(int64(n))
	}
	return n, err
}

// Write implements the io.Writer interface and tracks uploaded bytes.
func (mc *monitoredConn) Write(b []byte) (n int, err error) {
	n, err = mc.StreamConn.Write(b)
	if n > 0 {
		mc.stats.addUpload(int64(n))
	}
	return n, err
}

// Close closes the connection and updates connection statistics.
func (mc *monitoredConn) Close() error {
	mc.stats.connectionEnded(mc)
	return mc.StreamConn.Close()
}

// monitoredDialer wraps a transport.StreamDialer to create monitored connections.
type monitoredDialer struct {
	transport.StreamDialer
	stats *TrafficStats
}

// NewMonitoredDialer creates a dialer that produces monitored connections.
func NewMonitoredDialer(dialer transport.StreamDialer, stats *TrafficStats) transport.StreamDialer {
	return &monitoredDialer{
		StreamDialer: dialer,
		stats:        stats,
	}
}

// DialStream creates a monitored connection.
func (md *monitoredDialer) DialStream(ctx context.Context, address string) (transport.StreamConn, error) {
	conn, err := md.StreamDialer.DialStream(ctx, address)
	if err != nil {
		return nil, err
	}
	
	return NewMonitoredConn(conn, md.stats), nil
}

// MonitoredCopy copies data from src to dst while tracking traffic statistics.
// It's used to replace io.Copy calls in proxy handlers.
func MonitoredCopy(dst io.Writer, src io.Reader, stats *TrafficStats, isUpload bool) (written int64, err error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write")
				}
			}
			written += int64(nw)
			if nw > 0 {
				if isUpload {
					stats.addUpload(int64(nw))
				} else {
					stats.addDownload(int64(nw))
				}
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// Global traffic stats instance
var globalStats = NewTrafficStats()

// GetGlobalStats returns the global traffic statistics instance.
func GetGlobalStats() *TrafficStats {
	return globalStats
}