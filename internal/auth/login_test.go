package auth

import (
	"bytes"
	"encoding/json"
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

func withFastPoll(t *testing.T) {
	t.Helper()
	origInterval := pollInterval
	pollInterval = 50 * time.Millisecond
	t.Cleanup(func() { pollInterval = origInterval })
}

func TestStartAuthSession_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/auth/cli/start", r.URL.Path)

		var req cliStartRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "ABCD1234", req.Code)

		resp := cliStartResponse{CodeID: "code_id_12345"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	require.NoError(t, err)
	assert.Equal(t, "code_id_12345", codeID)
}

func TestStartAuthSession_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Contains(t, err.Error(), "status 500")
}

func TestStartAuthSession_InvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {"))
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Contains(t, err.Error(), "failed to parse auth start response")
}

func TestStartAuthSession_MissingCodeID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	require.NoError(t, err)
	assert.Equal(t, "", codeID)
}

func TestStartAuthSession_NetworkError(t *testing.T) {
	codeID, err := startAuthSession("http://invalid-host-that-does-not-exist-12345.local", "ABCD1234")
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Contains(t, err.Error(), "failed to start auth session")
}

func TestStartAuthSession_BadURL(t *testing.T) {
	codeID, err := startAuthSession("not a valid url", "ABCD1234")
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
}

func TestStartAuthSession_StatusUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	assert.Error(t, err)
	assert.Equal(t, "", codeID)
	assert.Contains(t, err.Error(), "status 401")
}

func TestPollForApproval_Approved(t *testing.T) {
	withFastPoll(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := cliPollResponse{
			Status:    "approved",
			Token:     "obt_token_123",
			Username:  "testuser",
			ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, err := pollForApproval(server.URL, "code_id_123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
	assert.Equal(t, "obt_token_123", result.Token)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, 1, callCount)
}

func TestPollForApproval_Expired(t *testing.T) {
	withFastPoll(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "expired"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, err := pollForApproval(server.URL, "code_id_123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestPollForApproval_Pending(t *testing.T) {
	withFastPoll(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			resp := cliPollResponse{Status: "pending"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	result, err := pollForApproval(server.URL, "code_id_123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
	assert.GreaterOrEqual(t, callCount, 3)
}

func TestPollForApproval_TimeoutBehavior(t *testing.T) {
	origTimeout := pollTimeout
	origInterval := pollInterval
	pollTimeout = 3 * time.Second
	pollInterval = 100 * time.Millisecond
	t.Cleanup(func() {
		pollTimeout = origTimeout
		pollInterval = origInterval
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "pending"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	start := time.Now()
	result, err := pollForApproval(server.URL, "code_id_123")
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timed out")
	assert.GreaterOrEqual(t, elapsed, 3*time.Second)
	assert.Less(t, elapsed, 5*time.Second)
}

func TestPollForApproval_InvalidResponse(t *testing.T) {
	origTimeout := pollTimeout
	origInterval := pollInterval
	pollTimeout = 3 * time.Second
	pollInterval = 100 * time.Millisecond
	t.Cleanup(func() {
		pollTimeout = origTimeout
		pollInterval = origInterval
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {"))
	}))
	defer server.Close()

	result, err := pollForApproval(server.URL, "code_id_123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timed out")
}

func TestPollOnce_Approved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{
			Status:    "approved",
			Token:     "obt_token_123",
			Username:  "testuser",
			ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, done, err := pollOnce(server.URL)
	require.NoError(t, err)
	assert.True(t, done)
	assert.NotNil(t, result)
	assert.Equal(t, "approved", result.Status)
}

func TestPollOnce_Pending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "pending"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, done, err := pollOnce(server.URL)
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestPollOnce_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "expired"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, done, err := pollOnce(server.URL)
	assert.Error(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestPollOnce_NetworkError(t *testing.T) {
	result, done, err := pollOnce("http://invalid-host-that-does-not-exist-12345.local")
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestPollOnce_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json {"))
	}))
	defer server.Close()

	result, done, err := pollOnce(server.URL)
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestLoginInteractive_SuccessRFC3339(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	startCalled := false
	pollCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			startCalled = true
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			pollCalled = true
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
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
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: "2026-01-15 10:00:00",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "obt_token_123", auth.Token)
	assert.Equal(t, "testuser", auth.Username)
}

func TestLoginInteractive_StartAuthSessionError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "status 500")
}

func TestLoginInteractive_PollForApprovalError(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{Status: "expired"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "authorization code expired")
}

func TestLoginInteractive_InvalidExpirationFormat(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: "invalid-date-format",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "failed to parse expiration time")
}

func TestLoginInteractive_SaveTokenError(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(authDir, 0500))
	t.Cleanup(func() {
		os.Chmod(authDir, 0700)
	})

	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	assert.Error(t, err)
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "failed to save auth token")
}

func TestLoginInteractive_CreatedAtTimestamp(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	before := time.Now()
	auth, err := LoginInteractive(server.URL)
	after := time.Now()

	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.True(t, auth.CreatedAt.After(before) || auth.CreatedAt.Equal(before))
	assert.True(t, auth.CreatedAt.Before(after) || auth.CreatedAt.Equal(after))
}

func TestLoginInteractive_TokenPersisted(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	require.NoError(t, err)

	loaded, err := LoadToken()
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, auth.Token, loaded.Token)
	assert.Equal(t, auth.Username, loaded.Username)
}

func TestGetAPIBase_DefaultValue(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "")
	base := GetAPIBase()
	assert.Equal(t, DefaultAPIBase, base)
}

func TestGetAPIBase_EnvOverride(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "https://custom.api.com")
	base := GetAPIBase()
	assert.Equal(t, "https://custom.api.com", base)
}

func TestLoginInteractive_MultiplePolls(t *testing.T) {
	withFastPoll(t)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			pollCount++
			if pollCount < 2 {
				resp := cliPollResponse{Status: "pending"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			} else {
				resp := cliPollResponse{
					Status:    "approved",
					Token:     "obt_token_123",
					Username:  "testuser",
					ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.GreaterOrEqual(t, pollCount, 2)
}

func TestStartAuthSession_ContentTypeHeader(t *testing.T) {
	headerReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerReceived = r.Header.Get("Content-Type")
		resp := cliStartResponse{CodeID: "code_id_123"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := startAuthSession(server.URL, "ABCD1234")
	require.NoError(t, err)
	assert.Equal(t, "application/json", headerReceived)
}

func TestPollForApproval_QueryParameter(t *testing.T) {
	withFastPoll(t)

	queryReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryReceived = r.URL.RawQuery
		resp := cliPollResponse{
			Status:    "approved",
			Token:     "obt_token_123",
			Username:  "testuser",
			ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := pollForApproval(server.URL, "code_id_123")
	require.NoError(t, err)
	assert.Contains(t, queryReceived, "code_id=code_id_123")
}

func TestLoginInteractive_ExpiresAtParsing(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	expectedExpiry := time.Date(2026, 1, 15, 10, 30, 45, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "testuser",
				ExpiresAt: expectedExpiry.Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	require.NoError(t, err)
	assert.Equal(t, expectedExpiry.Unix(), auth.ExpiresAt.Unix())
}

func TestStartAuthSession_RequestBody(t *testing.T) {
	bodyReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyReceived = string(body)
		resp := cliStartResponse{CodeID: "code_id_123"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := startAuthSession(server.URL, "TESTCODE")
	require.NoError(t, err)
	assert.Contains(t, bodyReceived, "TESTCODE")
}

func TestPollOnce_UnknownStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cliPollResponse{Status: "unknown_status"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, done, err := pollOnce(server.URL)
	assert.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, result)
}

func TestLoginInteractive_EmptyUsername(t *testing.T) {
	withFastPoll(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/cli/start" {
			resp := cliStartResponse{CodeID: "code_id_123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/auth/cli/poll" {
			resp := cliPollResponse{
				Status:    "approved",
				Token:     "obt_token_123",
				Username:  "",
				ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	auth, err := LoginInteractive(server.URL)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.Equal(t, "", auth.Username)
}

func TestStartAuthSession_LargeCodeID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		largeCodeID := bytes.Repeat([]byte("x"), 10000)
		resp := cliStartResponse{CodeID: string(largeCodeID)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	codeID, err := startAuthSession(server.URL, "ABCD1234")
	require.NoError(t, err)
	assert.Equal(t, 10000, len(codeID))
}
