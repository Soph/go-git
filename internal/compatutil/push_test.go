package compatutil

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashForPush(t *testing.T) {
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

	got, err := HashForPush(native, tr)
	require.NoError(t, err)
	assert.Equal(t, compat, got)

	zero, err := HashForPush(plumbing.ZeroHash, tr)
	require.NoError(t, err)
	assert.True(t, zero.IsZero())
}

func TestTranslatePushCommandsAndHashes(t *testing.T) {
	t.Parallel()

	st := memory.NewStorage(
		memory.WithObjectFormat(formatcfg.SHA256),
		memory.WithCompatObjectFormat(formatcfg.SHA1),
	)
	tr := st.Translator()
	require.NotNil(t, tr)

	native1 := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat1 := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	native2 := plumbing.NewHash("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	compat2 := plumbing.NewHash("dddddddddddddddddddddddddddddddddddddddd")
	require.NoError(t, tr.Mapping().Add(native1, compat1))
	require.NoError(t, tr.Mapping().Add(native2, compat2))

	cmds, err := TranslatePushCommands([]*packp.Command{{
		Name: "refs/heads/main",
		Old:  native1,
		New:  native2,
	}, {
		Name: "refs/tags/v1",
		Old:  plumbing.ZeroHash,
		New:  native1,
	}}, tr)
	require.NoError(t, err)
	require.Len(t, cmds, 2)
	assert.Equal(t, compat1, cmds[0].Old)
	assert.Equal(t, compat2, cmds[0].New)
	assert.True(t, cmds[1].Old.IsZero())
	assert.Equal(t, compat1, cmds[1].New)

	hashes, err := TranslatePushHashes([]plumbing.Hash{native1, plumbing.ZeroHash, native2}, tr)
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{compat1, plumbing.ZeroHash, compat2}, hashes)
}
