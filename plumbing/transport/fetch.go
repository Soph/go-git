package transport

import (
	"context"
	"io"

	"github.com/go-git/go-git/v6/plumbing/compat"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/utils/ioutil"
	xstorage "github.com/go-git/go-git/v6/x/storage"
)

// FetchPack fetches a packfile from the remote connection into the given
// storage repository and updates the shallow information.
func FetchPack(
	ctx context.Context,
	st storage.Storer,
	conn Connection,
	packf io.ReadCloser,
	shallowInfo *packp.ShallowUpdate,
	req *FetchRequest,
) (err error) {
	packf = ioutil.NewContextReadCloser(ctx, packf)

	// Do we have sideband enabled?
	var demuxer *sideband.Demuxer
	var reader io.Reader = packf
	caps := conn.Capabilities()
	if caps.Supports(capability.Sideband64k) {
		demuxer = sideband.NewDemuxer(sideband.Sideband64k, reader)
	} else if caps.Supports(capability.Sideband) {
		demuxer = sideband.NewDemuxer(sideband.Sideband, reader)
	}

	if demuxer != nil && req.Progress != nil {
		demuxer.Progress = req.Progress
		reader = demuxer
	}

	if tp, ok := st.(xstorage.CompatTranslatorProvider); ok {
		if t := tp.Translator(); t != nil {
			remoteFormat := remoteObjectFormat(conn)
			if remoteFormat == t.CompatObjectFormat() && remoteFormat != t.NativeObjectFormat() {
				tmp, cleanup, err := newCompatTempStorage(remoteFormat)
				if err != nil {
					return err
				}
				defer cleanup()
				if err := packfile.UpdateObjectStorage(tmp, reader); err != nil {
					return err
				}
				if err := compat.ImportStoredObjects(tmp, st, t); err != nil {
					return err
				}
			} else {
				if err := packfile.UpdateObjectStorage(st, reader); err != nil {
					return err
				}
				if err := compat.TranslateStoredObjects(st, t); err != nil {
					return err
				}
			}
		} else if err := packfile.UpdateObjectStorage(st, reader); err != nil {
			return err
		}
	} else if err := packfile.UpdateObjectStorage(st, reader); err != nil {
		return err
	}

	if err := packf.Close(); err != nil {
		return err
	}

	// Update shallow
	if shallowInfo != nil {
		if err := updateShallow(st, shallowInfo); err != nil {
			return err
		}
	}

	return nil
}

func remoteObjectFormat(conn Connection) formatcfg.ObjectFormat {
	caps := conn.Capabilities()
	if caps.Supports(capability.ObjectFormat) {
		if capValues := caps.Get(capability.ObjectFormat); len(capValues) > 0 {
			of := formatcfg.ObjectFormat(capValues[0])
			switch of {
			case formatcfg.SHA1, formatcfg.SHA256:
				return of
			}
		}
	}

	return formatcfg.DefaultObjectFormat
}

func updateShallow(st storage.Storer, shallowInfo *packp.ShallowUpdate) error {
	shallows, err := st.Shallow()
	if err != nil {
		return err
	}

outer:
	for _, s := range shallowInfo.Shallows {
		for _, oldS := range shallows {
			if s == oldS {
				continue outer
			}
		}
		shallows = append(shallows, s)
	}

	// unshallow commits
	for _, s := range shallowInfo.Unshallows {
		for i, oldS := range shallows {
			if s == oldS {
				shallows = append(shallows[:i], shallows[i+1:]...)
				break
			}
		}
	}

	return st.SetShallow(shallows)
}
