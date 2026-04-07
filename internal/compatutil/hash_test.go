package compatutil

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHash(t *testing.T) {
	t.Parallel()

	st := memory.NewStorage(
		memory.WithObjectFormat(formatcfg.SHA256),
		memory.WithCompatObjectFormat(formatcfg.SHA1),
	)
	tr := st.Translator()
	require.NotNil(t, tr)

	native := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.NoError(t, tr.Mapping().Add(native, compat))

	assert.Equal(t, native, NormalizeHash(tr, compat))
	assert.Equal(t, native, NormalizeHash(tr, native))
	assert.True(t, NormalizeHash(tr, plumbing.ZeroHash).IsZero())

	unknown := plumbing.NewHash("cccccccccccccccccccccccccccccccccccccccc")
	assert.Equal(t, unknown, NormalizeHash(tr, unknown))

	resolved, err := ResolveHash(tr, compat)
	require.NoError(t, err)
	assert.Equal(t, native, resolved)

	_, err = ResolveHash(tr, unknown)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)
}

func TestNormalizeStorageHashAndReference(t *testing.T) {
	t.Parallel()

	st := memory.NewStorage(
		memory.WithObjectFormat(formatcfg.SHA256),
		memory.WithCompatObjectFormat(formatcfg.SHA1),
	)
	tr := st.Translator()
	require.NotNil(t, tr)

	native := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.NoError(t, tr.Mapping().Add(native, compat))

	assert.Equal(t, native, NormalizeStorageHash(st, compat))
	resolved, err := ResolveStorageHash(st, compat)
	require.NoError(t, err)
	assert.Equal(t, native, resolved)

	ref := plumbing.NewHashReference("refs/heads/main", compat)
	normalized := NormalizeReference(st, ref)
	assert.Equal(t, ref.Name(), normalized.Name())
	assert.Equal(t, native, normalized.Hash())

	symbolic := plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")
	assert.Equal(t, symbolic, NormalizeReference(st, symbolic))

	unknown := plumbing.NewHash("cccccccccccccccccccccccccccccccccccccccc")
	_, err = ResolveStorageHash(st, unknown)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)
}
