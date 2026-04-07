package compatutil

import (
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/compat"
	"github.com/go-git/go-git/v6/storage"
	xstorage "github.com/go-git/go-git/v6/x/storage"
)

// NormalizeHash rewrites a compat hash to the native hash known by the
// translator. Unknown hashes are returned unchanged.
func NormalizeHash(t *compat.Translator, h plumbing.Hash) plumbing.Hash {
	if t == nil || h.IsZero() {
		return h
	}

	native, err := t.Mapping().CompatToNative(h)
	if err != nil {
		return h
	}

	return native
}

// NormalizeStorageHash rewrites a compat hash to the native hash known by the
// storage's translator, if one is configured.
func NormalizeStorageHash(s storage.Storer, h plumbing.Hash) plumbing.Hash {
	tp, ok := s.(xstorage.CompatTranslatorProvider)
	if !ok {
		return h
	}

	return NormalizeHash(tp.Translator(), h)
}

// NormalizeReference rewrites hash references from compat to native hashes
// using the storage's configured translator.
func NormalizeReference(s storage.Storer, ref *plumbing.Reference) *plumbing.Reference {
	if ref == nil || ref.Type() != plumbing.HashReference {
		return ref
	}

	return plumbing.NewHashReference(ref.Name(), NormalizeStorageHash(s, ref.Hash()))
}
