package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Transport mocks — no port binding.
// ---------------------------------------------------------------------------

type authMockRT struct{ handler http.Handler }

func (m *authMockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

type authErrRT struct{ err error }

func (e *authErrRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// withMockHTTPClient patches the package-level httpClient for the test.
func withMockHTTPClient(t *testing.T, handler http.Handler) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Transport: &authMockRT{handler: handler}}
	t.Cleanup(func() { httpClient = orig })
}

func withErrHTTPClient(t *testing.T, err error) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Transport: &authErrRT{err: err}}
	t.Cleanup(func() { httpClient = orig })
}

// fakeBase is a valid-looking https URL that the mock transport intercepts.
const fakeBase = "https://openboot.test"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func withFastPoll(t *testing.T) {
	t.Helper()
	origInterval := pollInterval
	pollInterval = 50 * time.Millisecond
	t.Cleanup(func() { pollInterval = origInterval })
}

func withNoBrowser(t *testing.T) {
	t.Helper()
	orig := openBrowserFunc
	openBrowserFunc = func(url string) error { return nil }
	t.Cleanup(func() { openBrowserFunc = orig })
}

// ---------------------------------------------------------------------------
// startAuthSession
// ---------------------------------------------------------------------------

func TestStartAuthSession_Success(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/auth/cli/start", r.URL.Path)
		resp := cliStartResponse{CodeID: "code_id_12345", Code: "ABCD1234"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	codeID, code, err := startAuthSession(fakeBase)
	require.NoError(t, err)
	assert.Equal(t, "code_id_12345", codeID)
	assert.Equal(t, "ABCD1234", code)
}

func TestStartAuthSession_ReturnsServerCode(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliStartResponse{CodeID: "some_id", Code: "SERVERCODE"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	_, code, err := startAuthSession(fakeBase)
	require.NoError(t, err)
	assert.Equal(t, "SERVERCODE", code, "code must come from the server, not be client-generated")
}

func TestStartAuthSession_NoRequestBody(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Empty(t, body, "start request must not send a body")
		resp := cliStartResponse{CodeID: "id", Code: "CODE1234"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	_, _, err := startAuthSession(fakeBase)
	require.NoError(t, err)
}

func TestStartAuthSession_HTTPError(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck // test helper
	}))

	codeID, code, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Equal(t, "", code)
	assert.Contains(t, err.Error(), "status 500")
}

func TestStartAuthSession_InvalidResponse(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {")) //nolint:errcheck // test helper
	}))

	codeID, code, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Equal(t, "", code)
	assert.Contains(t, err.Error(), "parse auth response")
}

func TestStartAuthSession_MissingCodeID(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"code": "ABCD1234"} // missing code_id
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	_, _, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestStartAuthSession_MissingCode(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"code_id": "some_id"} // missing code
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	_, _, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestStartAuthSession_NetworkError(t *testing.T) {
	withErrHTTPClient(t, errors.New("connection refused"))

	_, _, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "start auth session")
}

func TestStartAuthSession_BadURL(t *testing.T) {
	_, _, err := startAuthSession("not a valid url")
	assert.Error(t, err)
}

func TestStartAuthSession_StatusUnauthorized(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	_, _, err := startAuthSession(fakeBase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

// ---------------------------------------------------------------------------
// pollForApproval / pollOnce
// ---------------------------------------------------------------------------

func TestPollForApproval_Approved(t *testing.T) {
	withFastPoll(t)

	callCount := 0
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := cliPollResponse{
			Status:    "approved",
			Token:     "obt_token_123",
			Username:  "testuser",
			ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	result, err := pollForApproval(context.Background(), fakeBase+"/poll", "code_id_123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
	assert.Equal(t, "obt_token_123", result.Token)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, 1, callCount)
}

func TestPollForApproval_Expired(t *testing.T) {
	withFastPoll(t)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "expired"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	result, err := pollForApproval(context.Background(), fakeBase+"/poll", "code_id_123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestPollForApproval_Pending(t *testing.T) {
	withFastPoll(t)

	callCount := 0
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			resp := cliPollResponse{Status: "pending"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		} else {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	result, err := pollForApproval(context.Background(), fakeBase+"/poll", "code_id_123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
	assert.GreaterOrEqual(t, callCount, 3)
}

func TestPollForApproval_TimeoutBehavior(t *testing.T) {
	origTimeout := pollTimeout
	origInterval := pollInterval
	pollTimeout = 200 * time.Millisecond
	pollInterval = 20 * time.Millisecond
	t.Cleanup(func() {
		pollTimeout = origTimeout
		pollInterval = origInterval
	})

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "pending"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	start := time.Now()
	result, err := pollForApproval(context.Background(), fakeBase+"/poll", "code_id_123")
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timed out")
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond)
	assert.Less(t, elapsed, 1*time.Second)
}

func TestPollForApproval_InvalidResponse(t *testing.T) {
	origTimeout := pollTimeout
	origInterval := pollInterval
	pollTimeout = 200 * time.Millisecond
	pollInterval = 20 * time.Millisecond
	t.Cleanup(func() {
		pollTimeout = origTimeout
		pollInterval = origInterval
	})

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {")) //nolint:errcheck // test helper
	}))

	result, err := pollForApproval(context.Background(), fakeBase+"/poll", "code_id_123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timed out")
}

func TestPollOnce_Approved(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{
			Status:    "approved",
			Token:     "obt_token_123",
			Username:  "testuser",
			ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	result, done, err := pollOnce(fakeBase + "/poll/code_id_123")
	require.NoError(t, err)
	assert.True(t, done)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
}

func TestPollOnce_Pending(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "pending"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	result, done, err := pollOnce(fakeBase + "/poll/code_id_123")
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestPollOnce_Expired(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "expired"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))

	result, done, err := pollOnce(fakeBase + "/poll/code_id_123")
	assert.Error(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestPollOnce_NetworkError(t *testing.T) {
	// pollOnce swallows transport errors and returns (nil, false, nil).
	withErrHTTPClient(t, errors.New("connection refused"))

	result, done, err := pollOnce(fakeBase + "/poll/code_id_123")
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestPollOnce_InvalidJSON(t *testing.T) {
	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {")) //nolint:errcheck // test helper
	}))

	result, done, err := pollOnce(fakeBase + "/poll/code_id_123")
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// LoginInteractive
// ---------------------------------------------------------------------------

func TestLoginInteractive_SuccessRFC3339(t *testing.T) {
	withFastPoll(t)
	withNoBrowser(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	startCalled := false
	pollCalled := false

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/cli/start":
			startCalled = true
			resp := cliStartResponse{CodeID: "code_id_123", Code: "ABCD1234"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case "/api/auth/cli/poll":
			pollCalled = true
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "obt_token_123", auth.Token)
	assert.Equal(t, "testuser", auth.Username)
	assert.True(t, startCalled)
	assert.True(t, pollCalled)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	assert.FileExists(t, authFile)
}

func TestLoginInteractive_SuccessSQLiteFormat(t *testing.T) {
	withFastPoll(t)
	withNoBrowser(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/cli/start":
			resp := cliStartResponse{CodeID: "code_id_123", Code: "ABCD1234"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case "/api/auth/cli/poll":
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: "2026-01-15 10:00:00",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "obt_token_123", auth.Token)
	assert.Equal(t, "testuser", auth.Username)
}

func TestLoginInteractive_StartAuthSessionError(t *testing.T) {
	withNoBrowser(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "status 500")
}

func TestLoginInteractive_PollForApprovalError(t *testing.T) {
	withFastPoll(t)
	withNoBrowser(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/cli/start":
			resp := cliStartResponse{CodeID: "code_id_123", Code: "ABCD1234"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case "/api/auth/cli/poll":
			resp := cliPollResponse{Status: "expired"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestLoginInteractive_InvalidExpirationFormat(t *testing.T) {
	withFastPoll(t)
	withNoBrowser(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/cli/start":
			resp := cliStartResponse{CodeID: "code_id_123", Code: "ABCD1234"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case "/api/auth/cli/poll":
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: "invalid-date-format",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "parse expiration")
}

func TestLoginInteractive_SaveTokenError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem permission checks")
	}
	withFastPoll(t)
	withNoBrowser(t)
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(authDir, 0500))
	t.Cleanup(func() {
		os.Chmod(authDir, 0700) //nolint:errcheck // cleanup restore; failure is non-critical in tests
	})
	t.Setenv("HOME", tmpDir)

	withMockHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/cli/start":
			resp := cliStartResponse{CodeID: "code_id_123", Code: "ABCD1234"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case "/api/auth/cli/poll":
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		}
	}))

	auth, err := LoginInteractive(context.Background(), fakeBase)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "save auth token")
}
