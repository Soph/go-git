package compatutil

import (
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/compat"
	"github.com/go-git/go-git/v6/storage"
	xstorage "github.com/go-git/go-git/v6/x/storage"
)

// ResolveHash rewrites a compat hash to the native hash known by the
// translator. Unknown hashes return an error.
func ResolveHash(t *compat.Translator, h plumbing.Hash) (plumbing.Hash, error) {
	if t == nil || h.IsZero() {
		return h, nil
	}

	return t.Mapping().CompatToNative(h)
}

// NormalizeHash rewrites a compat hash to the native hash known by the
// translator. Unknown hashes are returned unchanged.
func NormalizeHash(t *compat.Translator, h plumbing.Hash) plumbing.Hash {
	native, err := ResolveHash(t, h)
	if err != nil {
		return h
	}
	return native
}

// ResolveStorageHash rewrites a compat hash to the native hash known by the
// storage's translator, if one is configured.
func ResolveStorageHash(s storage.Storer, h plumbing.Hash) (plumbing.Hash, error) {
	tp, ok := s.(xstorage.CompatTranslatorProvider)
	if !ok {
		return h, nil
	}

	return ResolveHash(tp.Translator(), h)
}

// NormalizeStorageHash rewrites a compat hash to the native hash known by the
// storage's translator, if one is configured.
func NormalizeStorageHash(s storage.Storer, h plumbing.Hash) plumbing.Hash {
	native, err := ResolveStorageHash(s, h)
	if err != nil {
		return h
	}
	return native
}

// NormalizeReference rewrites hash references from compat to native hashes
// using the storage's configured translator.
func NormalizeReference(s storage.Storer, ref *plumbing.Reference) *plumbing.Reference {
	if ref == nil || ref.Type() != plumbing.HashReference {
		return ref
	}

	return plumbing.NewHashReference(ref.Name(), NormalizeStorageHash(s, ref.Hash()))
}
