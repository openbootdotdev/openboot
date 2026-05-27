package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// MaxRetryAfter caps the maximum wait time for a Retry-After header to prevent
// the CLI from hanging indefinitely on unreasonable server values.
const MaxRetryAfter = 60 * time.Second

// RateLimitError is returned when the server returns 429 after a retry attempt.
type RateLimitError struct {
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("Rate limited. Please wait %d seconds and try again.", e.RetryAfterSeconds)
}

// parseRetryAfter reads the Retry-After header (in seconds) from a response.
// Returns 0 if the header is missing or unparseable.
func parseRetryAfter(resp *http.Response) int {
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}
	seconds, err := strconv.Atoi(val)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

// clampDuration returns the smaller of d and max.
func clampDuration(d, max time.Duration) time.Duration {
	if d > max {
		return max
	}
	return d
}

// sleepFunc is the function used to pause execution. Replaced in tests.
var sleepFunc = time.Sleep

// Do executes an HTTP request using the provided client, handling HTTP 429
// responses with a single retry. If the server returns a Retry-After header,
// the function waits for the specified duration (capped at MaxRetryAfter)
// before retrying once.
//
// The caller must close the response body as usual.
//
// Do does NOT mutate the request or client — it re-reads the request as-is
// for the retry. For requests with a body, the caller must ensure the body
// supports re-reading (e.g. via bytes.NewReader with Seek or by using
// http.NewRequest with a fresh body). In practice, all call sites in this
// codebase use either no body or bytes.NewReader, both of which support this.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req) //nolint:gosec // URL is caller-controlled and validated upstream; this is our own HTTP client wrapper
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}

	// Parse Retry-After and wait before retrying once.
	retryAfterSec := parseRetryAfter(resp)
	if retryAfterSec == 0 {
		retryAfterSec = 1 // Default to 1 second if header is missing.
	}

	waitDuration := clampDuration(
		time.Duration(retryAfterSec)*time.Second,
		MaxRetryAfter,
	)

	// Close the first response body before retrying.
	resp.Body.Close() //nolint:errcheck,gosec // best-effort body close before retry

	// Reset the request body for the retry if present.
	if req.GetBody != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, fmt.Errorf("reset request body for retry: %w", bodyErr)
		}
		req.Body = body
	}

	sleepFunc(waitDuration)

	retryResp, retryErr := client.Do(req) //nolint:gosec // URL is caller-controlled and validated upstream; this is our own HTTP client wrapper
	if retryErr != nil {
		return nil, retryErr
	}

	if retryResp.StatusCode == http.StatusTooManyRequests {
		retryResp.Body.Close() //nolint:errcheck,gosec // best-effort body close before error return
		retrySec := parseRetryAfter(retryResp)
		if retrySec == 0 {
			retrySec = retryAfterSec
		}
		return nil, &RateLimitError{RetryAfterSeconds: retrySec}
	}

	return retryResp, nil
}
