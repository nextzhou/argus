package assets

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAsset(t *testing.T) {
	data, err := ReadAsset("skills/README.md")
	require.NoError(t, err)
	assert.Contains(t, string(data), "Built-in skill definitions")
}

func TestReadAssetNotFound(t *testing.T) {
	_, err := ReadAsset("nonexistent/file.txt")
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading asset")
}

func TestListAssets(t *testing.T) {
	names, err := ListAssets("skills")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(names), 1)
	assert.Contains(t, names, "README.md")
}

func TestWalkAssets(t *testing.T) {
	var count int
	err := WalkAssets("skills", func(_ string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)
}
