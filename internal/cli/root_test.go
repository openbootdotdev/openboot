package cli

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentPreRunE_SetsVersion(t *testing.T) {
	orig := installCfg
	installCfg = &config.Config{}
	t.Cleanup(func() { installCfg = orig })

	version = "1.2.3"
	t.Cleanup(func() { version = "dev" })

	err := rootCmd.PersistentPreRunE(rootCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", installCfg.Version)
}
