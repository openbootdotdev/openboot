package httputil

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRT intercepts requests in-memory — no port binding.
type mockRT struct{ handler http.Handler }

func (m *mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

type errRT struct{ err error }

func (e *errRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func mockClient(h http.Handler) *http.Client { return &http.Client{Transport: &mockRT{handler: h}} }
func errClient(err error) *http.Client       { return &http.Client{Transport: &errRT{err: err}} }

const fakeURL = "https://openboot.test/endpoint"

func TestDo_NoRateLimit(t *testing.T) {
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))
}

func TestDo_RateLimitThenSuccess(t *testing.T) {
	var calls atomic.Int32
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited")) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success")) //nolint:errcheck
	}))

	var sleptDuration time.Duration
	originalSleep := sleepFunc
	sleepFunc = func(d time.Duration) { sleptDuration = d }
	defer func() { sleepFunc = originalSleep }()

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2*time.Second, sleptDuration)
	assert.Equal(t, int32(2), calls.Load())
}

func TestDo_RateLimitTwiceReturnsError(t *testing.T) {
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	originalSleep := sleepFunc
	sleepFunc = func(d time.Duration) {}
	defer func() { sleepFunc = originalSleep }()

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	assert.Nil(t, resp)
	require.Error(t, err)

	var rateLimitErr *RateLimitError
	require.ErrorAs(t, err, &rateLimitErr)
	assert.Equal(t, 30, rateLimitErr.RetryAfterSeconds)
	assert.Contains(t, rateLimitErr.Error(), "Rate limited")
	assert.Contains(t, rateLimitErr.Error(), "30 seconds")
}

func TestDo_RetryAfterCappedAtMax(t *testing.T) {
	var calls atomic.Int32
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	var sleptDuration time.Duration
	originalSleep := sleepFunc
	sleepFunc = func(d time.Duration) { sleptDuration = d }
	defer func() { sleepFunc = originalSleep }()

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, MaxRetryAfter, sleptDuration)
}

func TestDo_MissingRetryAfterDefaultsToOneSecond(t *testing.T) {
	var calls atomic.Int32
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	var sleptDuration time.Duration
	originalSleep := sleepFunc
	sleepFunc = func(d time.Duration) { sleptDuration = d }
	defer func() { sleepFunc = originalSleep }()

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, 1*time.Second, sleptDuration)
}

func TestDo_WithRequestBody(t *testing.T) {
	var calls atomic.Int32
	var receivedBodies []string
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	originalSleep := sleepFunc
	sleepFunc = func(d time.Duration) {}
	defer func() { sleepFunc = originalSleep }()

	payload := []byte(`{"key":"value"}`)
	req, err := http.NewRequest("POST", fakeURL, bytes.NewReader(payload))
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), calls.Load())
	require.Len(t, receivedBodies, 2)
	assert.Equal(t, string(payload), receivedBodies[0])
	assert.Equal(t, string(payload), receivedBodies[1])
}

func TestDo_NetworkError(t *testing.T) {
	client := errClient(errors.New("connection refused"))
	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	assert.Nil(t, resp)
	require.Error(t, err)
}

func TestDo_OtherErrorStatusNotRetried(t *testing.T) {
	var calls atomic.Int32
	client := mockClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck
	}))

	req, err := http.NewRequest("GET", fakeURL, nil)
	require.NoError(t, err)

	resp, err := Do(client, req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, int32(1), calls.Load())
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected int
	}{
		{"valid number", "5", 5},
		{"zero", "0", 0},
		{"negative", "-1", 0},
		{"empty", "", 0},
		{"non-numeric", "abc", 0},
		{"large value", strconv.Itoa(3600), 3600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			assert.Equal(t, tt.expected, parseRetryAfter(resp))
		})
	}
}

func TestClampDuration(t *testing.T) {
	assert.Equal(t, 5*time.Second, clampDuration(5*time.Second, 60*time.Second))
	assert.Equal(t, 60*time.Second, clampDuration(120*time.Second, 60*time.Second))
	assert.Equal(t, 60*time.Second, clampDuration(60*time.Second, 60*time.Second))
}
