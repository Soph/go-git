package compat

import "github.com/go-git/go-git/v6/plumbing"

type encodedObjectSetter interface {
	SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
}

type encodedObjectLookup interface {
	HasEncodedObject(plumbing.Hash) error
	EncodedObject(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error)
}

// HasEncodedObject returns nil if the object exists by native or compat hash.
func HasEncodedObject(s encodedObjectLookup, t *Translator, h plumbing.Hash) error {
	err := s.HasEncodedObject(h)
	if err == nil || t == nil {
		return err
	}

	native, cerr := t.Mapping().CompatToNative(h)
	if cerr != nil {
		return err
	}
	return s.HasEncodedObject(native)
}

// EncodedObject returns the object by native or compat hash.
func EncodedObject(s encodedObjectLookup, t *Translator, ot plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	obj, err := s.EncodedObject(ot, h)
	if err == nil || t == nil {
		return obj, err
	}

	native, cerr := t.Mapping().CompatToNative(h)
	if cerr != nil {
		return nil, err
	}
	return s.EncodedObject(ot, native)
}

// SetEncodedObject stores the object and records its compat mapping.
//
// Normal writes are strict about signaling translation failures: if compat
// mapping cannot be computed immediately, this returns that error to the caller
// instead of silently suppressing it. The underlying object may already have
// been persisted by the wrapped storage before the translation error occurs.
//
// Batch import paths may opt into deferred mapping creation by passing
// deferMapping=true. In that mode the object is stored without attempting
// translation, and the caller is responsible for completing the backfill later
// via TranslateStoredObjects.
func SetEncodedObject(s encodedObjectSetter, t *Translator, obj plumbing.EncodedObject, deferMapping bool) (plumbing.Hash, error) {
	h, err := s.SetEncodedObject(obj)
	if err != nil || t == nil || deferMapping {
		return h, err
	}

	_, err = t.TranslateObject(obj)
	return h, err
}
