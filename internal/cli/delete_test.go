package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteCmd_CommandStructure(t *testing.T) {
	assert.Equal(t, "delete <slug>", deleteCmd.Use)
	assert.NotEmpty(t, deleteCmd.Short)
	assert.NotEmpty(t, deleteCmd.Long)
	assert.NotEmpty(t, deleteCmd.Example)
	assert.NotNil(t, deleteCmd.RunE)

	flag := deleteCmd.Flags().Lookup("force")
	assert.NotNil(t, flag, "should have --force flag")
	assert.Equal(t, "f", flag.Shorthand)
	assert.Equal(t, "false", flag.DefValue)
}

func TestDeleteCmd_RequiresSlugArg(t *testing.T) {
	err := deleteCmd.Args(deleteCmd, []string{})
	assert.Error(t, err, "should require exactly one argument")

	err = deleteCmd.Args(deleteCmd, []string{"my-slug"})
	assert.NoError(t, err, "should accept exactly one argument")

	err = deleteCmd.Args(deleteCmd, []string{"slug1", "slug2"})
	assert.Error(t, err, "should reject more than one argument")
}

func TestRunDelete_NotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)

	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runDelete("test-slug", true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestRunDelete_Success(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/configs/my-config", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runDelete("my-config", true)
	assert.NoError(t, err)
}

func TestRunDelete_NotFound(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runDelete("nonexistent", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunDelete_Unauthorized(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runDelete("my-config", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
}

func TestRunDelete_ServerError(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runDelete("my-config", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed (status 500)")
}
