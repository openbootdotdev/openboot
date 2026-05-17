package httputil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadReturnsContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "HEAD", r.Method)
		w.Header().Set("Content-Length", "12345")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := Head(context.Background(), http.DefaultClient, srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int64(12345), resp.ContentLength)
}

func TestHeadWithBearerSendsAuthorization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "HEAD", r.Method)
		assert.Equal(t, "Bearer QQ==", r.Header.Get("Authorization"))
		w.Header().Set("Content-Length", "424157")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := HeadWithBearer(context.Background(), http.DefaultClient, srv.URL, "QQ==")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int64(424157), resp.ContentLength)
}

func TestHeadFollowsRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "99")
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	resp, err := Head(context.Background(), http.DefaultClient, redirect.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int64(99), resp.ContentLength)
}
