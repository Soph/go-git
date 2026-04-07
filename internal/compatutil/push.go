package compatutil

import (
	"fmt"
	"io"

	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/compat"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	"github.com/go-git/go-git/v6/storage"
)

// TranslatePushCommands rewrites push commands from native to compat hashes.
func TranslatePushCommands(cmds []*packp.Command, t *compat.Translator) ([]*packp.Command, error) {
	translated := make([]*packp.Command, 0, len(cmds))
	for _, cmd := range cmds {
		oldHash, err := HashForPush(cmd.Old, t)
		if err != nil {
			return nil, err
		}
		newHash, err := HashForPush(cmd.New, t)
		if err != nil {
			return nil, err
		}
		translated = append(translated, &packp.Command{
			Name: cmd.Name,
			Old:  oldHash,
			New:  newHash,
		})
	}
	return translated, nil
}

// TranslatePushHashes rewrites the wants/haves in a push negotiation from
// native to compat hashes.
func TranslatePushHashes(hs []plumbing.Hash, t *compat.Translator) ([]plumbing.Hash, error) {
	translated := make([]plumbing.Hash, 0, len(hs))
	for _, h := range hs {
		compatHash, err := HashForPush(h, t)
		if err != nil {
			return nil, err
		}
		translated = append(translated, compatHash)
	}
	return translated, nil
}

// HashForPush rewrites a native object hash to the compat hash expected by
// the remote side of a cross-format push.
func HashForPush(h plumbing.Hash, t *compat.Translator) (plumbing.Hash, error) {
	if h.IsZero() {
		return h, nil
	}
	compatHash, err := t.Mapping().NativeToCompat(h)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("resolve compat hash for %s: %w", h, err)
	}
	return compatHash, nil
}

// NewPushStorer returns a storer view that exposes objects in compat format
// for a cross-format push while keeping the backing storage in native format.
func NewPushStorer(st storage.Storer, t *compat.Translator) storage.Storer {
	return &pushStorer{Storer: st, translator: t}
}

type pushStorer struct {
	storage.Storer
	translator *compat.Translator
}

func (s *pushStorer) Config() (*config.Config, error) {
	cfg, err := s.Storer.Config()
	if err != nil {
		return nil, err
	}

	cloned := *cfg
	cloned.Extensions = cfg.Extensions
	cloned.Extensions.ObjectFormat = s.translator.CompatObjectFormat()
	return &cloned, nil
}

func (s *pushStorer) NewEncodedObject() plumbing.EncodedObject {
	return plumbing.NewMemoryObject(plumbing.FromObjectFormat(s.translator.CompatObjectFormat()))
}

func (s *pushStorer) EncodedObject(ot plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	nativeHash, err := s.translator.Mapping().CompatToNative(h)
	if err != nil {
		return nil, plumbing.ErrObjectNotFound
	}

	obj, err := s.Storer.EncodedObject(ot, nativeHash)
	if err != nil {
		return nil, err
	}

	compatContent, err := pushObjectContent(obj, s.translator)
	if err != nil {
		return nil, err
	}

	translated := plumbing.NewMemoryObject(plumbing.FromObjectFormat(s.translator.CompatObjectFormat()))
	translated.SetType(obj.Type())
	translated.SetSize(int64(len(compatContent)))
	w, err := translated.Writer()
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(compatContent); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return translated, nil
}

func pushObjectContent(obj plumbing.EncodedObject, t *compat.Translator) ([]byte, error) {
	reader, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return t.ReverseTranslateContent(obj.Type(), content)
}
