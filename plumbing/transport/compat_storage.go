package transport

import (
	"os"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6/plumbing/cache"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/storage/filesystem"
)

func newCompatTempStorage(objectFormat formatcfg.ObjectFormat) (*filesystem.Storage, func(), error) {
	tmpDir, err := os.MkdirTemp("", "go-git-compat-*")
	if err != nil {
		return nil, nil, err
	}

	st := filesystem.NewStorageWithOptions(
		osfs.New(tmpDir, osfs.WithBoundOS()),
		cache.NewObjectLRUDefault(),
		filesystem.Options{ObjectFormat: objectFormat},
	)
	if err := st.Init(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, err
	}

	return st, func() {
		_ = os.RemoveAll(tmpDir)
	}, nil
}
