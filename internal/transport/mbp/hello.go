package mbp

import (
	"fmt"
)

// ServerCapabilities lists all capabilities this server advertises.
var ServerCapabilities = []string{"compression", "streaming", "subscriptions"}

// ServerLimits defines the server's operational constraints.
var ServerLimits = Limits{
	MaxResults:   100,
	MaxHopDepth:  5,
	MaxRate:      10,
	MaxPayloadMB: 16,
}

// ValidateHelloRequest validates a HELLO request payload.
func ValidateHelloRequest(req *HelloRequest) error {
	if req.Version != "1" {
		return fmt.Errorf("invalid version: expected 1, got %s", req.Version)
	}

	if req.AuthMethod == "" {
		req.AuthMethod = "none" // default
	}

	if req.AuthMethod != "token" && req.AuthMethod != "none" {
		return fmt.Errorf("invalid auth_method: %s", req.AuthMethod)
	}

	// If auth_method is "token", token must not be empty
	if req.AuthMethod == "token" && req.Token == "" {
		return fmt.Errorf("token required when auth_method is token")
	}

	return nil
}

// NegotiateCapabilities returns the intersection of client and server capabilities.
func NegotiateCapabilities(clientCapabilities []string) []string {
	if len(clientCapabilities) == 0 {
		return []string{} // Client advertises nothing, server provides nothing
	}

	clientSet := make(map[string]bool)
	for _, cap := range clientCapabilities {
		clientSet[cap] = true
	}

	var result []string
	for _, serverCap := range ServerCapabilities {
		if clientSet[serverCap] {
			result = append(result, serverCap)
		}
	}

	return result
}

// BuildHelloResponse constructs a HELLO_OK response.
func BuildHelloResponse(sessionID, vaultID string, clientCapabilities []string) *HelloResponse {
	negotiatedCaps := NegotiateCapabilities(clientCapabilities)

	return &HelloResponse{
		ServerVersion: "1.0.0",
		SessionID:     sessionID,
		VaultID:       vaultID,
		Capabilities:  negotiatedCaps,
		Limits:        ServerLimits,
	}
}

// BuildErrorPayload constructs an error response.
func BuildErrorPayload(code ErrorCode, message string) *ErrorPayload {
	return &ErrorPayload{
		Code:    code,
		Message: message,
	}
}

// BuildErrorPayloadWithID constructs an error response with request ID.
func BuildErrorPayloadWithID(code ErrorCode, message, requestID string) *ErrorPayload {
	return &ErrorPayload{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	}
}

// BuildErrorPayloadWithRetry constructs a rate-limit error with retry-after.
func BuildErrorPayloadWithRetry(code ErrorCode, message string, retryAfter int) *ErrorPayload {
	return &ErrorPayload{
		Code:       code,
		Message:    message,
		RetryAfter: retryAfter,
	}
}
