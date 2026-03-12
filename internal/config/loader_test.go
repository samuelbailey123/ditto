package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataPath returns the absolute path to the top-level testdata directory,
// resolving upward from the location of this source file so the tests work
// regardless of the working directory set by the test runner.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must succeed")
	// internal/config/ -> project root is two levels up
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", name)
}

func TestLoadFile_Basic(t *testing.T) {
	path := testdataPath(t, "basic.yaml")

	mf, err := config.LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "basic-api", mf.Name)
	assert.Len(t, mf.Routes, 5)

	first := mf.Routes[0]
	assert.Equal(t, "GET", first.Method)
	assert.Equal(t, "/health", first.Path)
	assert.Equal(t, 200, first.Status)
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := config.LoadFile("/does/not/exist/mock.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "bad-*.yaml")
	require.NoError(t, err)

	_, err = f.WriteString("{ this: is: not: valid: yaml: [\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = config.LoadFile(f.Name())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing file")
}

func TestLoadFiles_Multiple(t *testing.T) {
	basicPath := testdataPath(t, "basic.yaml")
	chaosPath := testdataPath(t, "chaos.yaml")

	cfg, err := config.LoadFiles(basicPath, chaosPath)
	require.NoError(t, err)

	// basic.yaml has 5 routes, chaos.yaml has 2 routes.
	assert.Len(t, cfg.Routes, 7)
}

func TestLoadFiles_Defaults(t *testing.T) {
	path := testdataPath(t, "basic.yaml")

	cfg, err := config.LoadFiles(path)
	require.NoError(t, err)

	require.NotNil(t, cfg.Defaults.Headers)
	assert.Equal(t, "application/json", cfg.Defaults.Headers["Content-Type"])
}

func TestLoadFiles_NoPaths(t *testing.T) {
	_, err := config.LoadFiles()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no files provided")
}

func TestLoadFiles_PropagatesFileError(t *testing.T) {
	_, err := config.LoadFiles("/does/not/exist.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestMergeConfigs_NilFileSkipped(t *testing.T) {
	basicPath := testdataPath(t, "basic.yaml")
	mf, err := config.LoadFile(basicPath)
	require.NoError(t, err)

	cfg := config.MergeConfigs(mf, nil)
	assert.Len(t, cfg.Routes, 5)
}
