package millionsend

import (
	"encoding/json"
	"fmt"
)

// MillionSendError is returned for every non-2xx API response and for
// client-side/transport failures. It implements the error interface, and its
// Name is a stable, switchable code (e.g. "validation_error", "not_found",
// "restricted_api_key", "sending_paused", "all_recipients_suppressed").
type MillionSendError struct {
	// StatusCode is the HTTP status. It is 0 when the request never reached the
	// API (a transport or client-side failure) — the wire's "statusCode: null".
	StatusCode int `json:"statusCode"`
	// Name is the stable discriminant you can switch on. Message is for
	// people: don't parse it.
	Name    string `json:"name"`
	Message string `json:"message"`
	// RetryAfter is the response's retry-after header in seconds, nil when it
	// has none. A 429 rate_limit_exceeded can last until the next UTC
	// midnight, so read it before retrying rather than the name alone.
	RetryAfter *int `json:"-"`
}

func (e *MillionSendError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("millionsend: %s: %s", e.Name, e.Message)
	}
	return fmt.Sprintf("millionsend: %d %s: %s", e.StatusCode, e.Name, e.Message)
}

// parseError coerces a non-2xx body into a *MillionSendError, falling back to a
// generic shape when the body is not the canonical { statusCode, name, message }.
func parseError(body []byte, status int) *MillionSendError {
	var e MillionSendError
	if len(body) > 0 && json.Unmarshal(body, &e) == nil && e.Name != "" {
		if e.StatusCode == 0 {
			e.StatusCode = status
		}
		return &e
	}
	return &MillionSendError{
		StatusCode: status,
		Name:       "application_error",
		Message:    fmt.Sprintf("request failed with status %d", status),
	}
}
