# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

The Outline SDK is a network interference protection toolkit developed by Jigsaw (Google) that helps create tools to bypass network-level censorship and blocking. The project is structured as a Go-based SDK with mobile bindings and command-line utilities.

## Key Commands

### Building
```bash
# Build main SDK
go build -v ./...

# Build experimental features with all tags
go build -C x -tags psiphon -o out/ -v ./...

# Build mobile libraries (requires Task runner)
task build:mobileproxy              # Both iOS and Android
task build:mobileproxy:ios          # iOS only
task build:mobileproxy:android      # Android only
```

### Testing
```bash
# Run tests for main SDK
go test -race ./...

# Run tests with network integration tests
go test -tags nettest -race -bench '.' ./... -benchtime=100ms

# Run tests for experimental features
go test -C x -tags nettest,psiphon -race -bench '.' ./... -benchtime=100ms
```

### License Checking
```bash
# Check SDK licenses
go run github.com/google/go-licenses check --ignore=golang.org/x --allowed_licenses=Apache-2.0,Apache-3,BSD-3-Clause,BSD-4-Clause,CC0-1.0,MIT ./...

# Check experimental features licenses
go run github.com/google/go-licenses check --ignore=golang.org/x --allowed_licenses=Apache-2.0,Apache-3,BSD-3-Clause,BSD-4-Clause,CC0-1.0,MIT ./...
```

## Architecture

### Core SDK Structure (`/`)
- **`transport/`** - Core transport layer with connections and dialers
  - `shadowsocks/` - Shadowsocks proxy protocol implementation
  - `socks5/` - SOCKS5 proxy protocol implementation
  - `tls/` - TLS transport with SNI manipulation capabilities
  - `tlsfrag/` - TLS record fragmentation for DPI evasion
  - `split/` - TCP stream fragmentation strategies
- **`network/`** - Network layer (OSI layer 3) functionality
  - `lwip2transport/` - User-space network stack implementation
  - `dnstruncate/` - DNS packet truncation utilities
- **`dns/`** - DNS resolution utilities with encrypted DNS support
- **`internal/`** - Internal utilities and shared code

### Experimental Features (`x/`)
- **`configurl/`** - Flexible configuration system using URL-like strings
- **`mobileproxy/`** - Mobile proxy library for iOS/Android integration
- **`examples/`** - Comprehensive example applications and tools
- **`smart/`** - Smart proxy that automatically selects working strategies
- **`psiphon/`** - Psiphon integration support
- **`connectivity/`** - Network connectivity testing utilities

### Go Modules
- **Main module**: `github.com/Jigsaw-Code/outline-sdk` (Go 1.20+)
- **Experimental module**: `github.com/Jigsaw-Code/outline-sdk/x` (Go 1.23+)

### Core Concepts
1. **Connections**: Stream (TCP-like) and Packet (UDP-like) connections
2. **Dialers**: Factory interfaces for creating connections with different transports
3. **Resolvers**: DNS resolution with multiple transport options
4. **Composability**: Transports can be nested and combined

### Transport Configuration
Uses URL-like strings for transport configuration:
```bash
# Examples
ss://userinfo@host:port          # Shadowsocks
socks5://user:pass@host:port     # SOCKS5
tls:sni=example.com             # TLS with SNI
split:2|tlsfrag:123             # Combined strategies
```

## Command-Line Tools

The SDK provides several utilities in `x/examples/`:
- **`resolve`** - DNS resolution tool
- **`fetch`** - HTTP client with transport configuration
- **`http2transport`** - Local proxy server
- **`test-connectivity`** - Proxy connectivity testing
- **`fetch-speed`** - Download speed testing
- **`outline-cli`** - Full VPN client implementation

Run tools with:
```bash
go run github.com/Jigsaw-Code/outline-sdk/x/examples/[tool]@latest [args...]
```

## Mobile Integration

### Task Runner
Uses `task` command for mobile builds from `x/Taskfile.yml`:
- `task build:mobileproxy` - Build both iOS and Android
- `task clean` - Clean build artifacts

### Requirements
- Go Mobile tools (automatically installed by task)
- Android SDK for Android builds
- Xcode for iOS builds

## Development Notes

### Multi-Platform Support
- Supports Android, iOS, Windows, macOS, Linux
- CI runs on Ubuntu, macOS, and Windows
- Mobile library builds in CI pipeline

### Network Tests
- Use `-tags nettest` for tests that require network access
- Tests include external network requests and connectivity validation
- Benchmarks included with `-bench '.' -benchtime=100ms`

### Integration Methods
1. **Generated Mobile Library**: For iOS/Android apps using gomobile
2. **Side Service**: Standalone Go binary for desktop applications
3. **Go Library**: Direct integration for Go applications
4. **Generated C Library**: C bindings for other languages

### Build Tags
- `psiphon` - Enables Psiphon integration features
- `nettest` - Enables network-dependent tests