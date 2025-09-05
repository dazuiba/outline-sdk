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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrorCode represents connectivity test error codes, compatible with Outline app error codes.
type ErrorCode = string

// Connectivity test error codes matching Outline app's platerrors package
const (
	// ConnectivityTestSuccess indicates successful connectivity test
	ConnectivityTestSuccess ErrorCode = "CONNECTIVITY_TEST_SUCCESS"
	
	// ProxyServerUnreachable means we failed to establish a connection to the proxy server
	ProxyServerUnreachable ErrorCode = "ERR_PROXY_SERVER_UNREACHABLE"
	
	// ProxyServerWriteFailed means we failed to write data to the proxy server
	ProxyServerWriteFailed ErrorCode = "ERR_PROXY_SERVER_WRITE_FAILURE"
	
	// ProxyServerReadFailed means we failed to read data from the proxy server
	ProxyServerReadFailed ErrorCode = "ERR_PROXY_SERVER_READ_FAILURE"
	
	// ClientUnauthenticated indicates authentication failure with the proxy server
	ClientUnauthenticated ErrorCode = "ERR_CLIENT_UNAUTHENTICATED"
	
	// IllegalConfig indicates an invalid configuration
	IllegalConfig ErrorCode = "ERR_ILLEGAL_CONFIG"
	
	// InternalError represents a general internal error
	InternalError ErrorCode = "ERR_INTERNAL_ERROR"
	
	// TimeoutError indicates the operation timed out
	TimeoutError ErrorCode = "ERR_TIMEOUT"
	
	// ResolveIPFailed means DNS resolution failed
	ResolveIPFailed ErrorCode = "ERR_RESOLVE_IP_FAILURE"
)

// ErrorDetails represents structured technical details in a ConnectivityError.
type ErrorDetails = map[string]interface{}

// ConnectivityError represents a connectivity test error that can be serialized to JSON
// and shared between Go and client code (Swift/Java).
type ConnectivityError struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details ErrorDetails   `json:"details,omitempty"`
	Cause   *ConnectivityError `json:"cause,omitempty"`
}

var _ error = ConnectivityError{}

// NewConnectivityError creates a new ConnectivityError from the error code and message.
func NewConnectivityError(code ErrorCode, message string) *ConnectivityError {
	return &ConnectivityError{
		Code:    code,
		Message: message,
	}
}

// NewConnectivityErrorWithDetails creates a new ConnectivityError with additional details.
func NewConnectivityErrorWithDetails(code ErrorCode, message string, details ErrorDetails) *ConnectivityError {
	return &ConnectivityError{
		Code:    code,
		Message: message,
		Details: details,
	}
}

// ToConnectivityError converts an error into a ConnectivityError.
// If the provided err is already a ConnectivityError, it is returned as is.
// Otherwise, the err is wrapped into a new ConnectivityError of InternalError.
// It returns nil if err is nil.
func ToConnectivityError(err error) *ConnectivityError {
	if err == nil {
		return nil
	}
	if ce, ok := err.(ConnectivityError); ok {
		ce.normalize()
		return &ce
	}
	if ce, ok := err.(*ConnectivityError); ok {
		if ce == nil {
			return nil
		}
		ce.normalize()
		return ce
	}
	
	// Map common error types to specific error codes
	errMsg := err.Error()
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return &ConnectivityError{Code: TimeoutError, Message: errMsg}
	}
	if strings.Contains(errMsg, "connection refused") {
		return &ConnectivityError{Code: ProxyServerUnreachable, Message: errMsg}
	}
	if strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "dns") {
		return &ConnectivityError{Code: ResolveIPFailed, Message: errMsg}
	}
	
	return &ConnectivityError{Code: InternalError, Message: errMsg}
}

// Error returns a string representation of the error.
func (e ConnectivityError) Error() string {
	e.normalize()
	msg := fmt.Sprintf("(%v) %v", e.Code, e.Message)
	if e.Cause != nil {
		msg += fmt.Sprintf(": %v", e.Cause)
	}
	return msg
}

// Unwrap returns the cause of this ConnectivityError.
func (e ConnectivityError) Unwrap() error {
	return e.Cause
}

// ToJSON returns a JSON string representation of the ConnectivityError.
// The resulting JSON can be used to reconstruct the error in client code.
func (e *ConnectivityError) ToJSON() (string, error) {
	if e == nil {
		return "", errors.New("a non-nil ConnectivityError is required")
	}
	e.normalize()
	jsonBytes, err := json.Marshal(e)
	return string(jsonBytes), err
}

// normalize ensures that all fields in the ConnectivityError are valid.
// It sets a default value if e.Code is empty.
func (e *ConnectivityError) normalize() {
	if strings.TrimSpace(string(e.Code)) == "" {
		e.Code = InternalError
	}
}

// WithCause adds a cause to the ConnectivityError.
func (e *ConnectivityError) WithCause(cause error) *ConnectivityError {
	e.Cause = ToConnectivityError(cause)
	return e
}

// WithDetails adds details to the ConnectivityError.
func (e *ConnectivityError) WithDetails(details ErrorDetails) *ConnectivityError {
	e.Details = details
	return e
}