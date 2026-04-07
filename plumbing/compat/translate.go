package compat

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v6/plumbing"
	format "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// ErrMissingDependencyMapping indicates that translation could not proceed
// because a referenced object does not yet have a compat/native mapping.
var ErrMissingDependencyMapping = errors.New("missing dependency mapping")

// Translator converts objects between native and compat hash formats,
// computing the compat-format hash and recording the mapping.
//
// Objects must be translated in topological order: blobs first, then
// trees, then commits and tags. Each object's referenced objects must
// already have mappings recorded before the object itself is translated.
type Translator struct {
	formats      Formats
	nativeHasher *plumbing.ObjectHasher
	compatHasher *plumbing.ObjectHasher
	mapping      HashMapping
}

// NewTranslator creates a Translator for the given format pair and mapping.
func NewTranslator(f Formats, m HashMapping) *Translator {
	return &Translator{
		formats:      f,
		nativeHasher: plumbing.FromObjectFormat(f.Native),
		compatHasher: plumbing.FromObjectFormat(f.Compat),
		mapping:      m,
	}
}

// Mapping returns the underlying HashMapping.
func (t *Translator) Mapping() HashMapping {
	return t.mapping
}

// TranslateObject computes the compat-format hash for an object stored in
// native format. It translates internal hash references (in trees, commits,
// and tags) using the mapping, then hashes the translated content with the
// compat hasher. The resulting mapping is recorded.
//
// For blobs, content is identical across formats; only the hash differs.
func (t *Translator) TranslateObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	if obj.Type() == plumbing.BlobObject {
		compatHash, err := t.computeBlobCompatHash(obj)
		if err != nil {
			return plumbing.Hash{}, err
		}
		if err := t.mapping.Add(obj.Hash(), compatHash); err != nil {
			return plumbing.Hash{}, fmt.Errorf("record mapping: %w", err)
		}
		return compatHash, nil
	}

	content, err := readObjectContent(obj)
	if err != nil {
		return plumbing.Hash{}, err
	}

	compatContent, err := t.rewriteContentToCompat(obj.Type(), content)
	if err != nil {
		return plumbing.Hash{}, err
	}

	compatHash, err := t.compatHasher.Compute(obj.Type(), compatContent)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("compute compat hash: %w", err)
	}

	if err := t.mapping.Add(obj.Hash(), compatHash); err != nil {
		return plumbing.Hash{}, fmt.Errorf("record mapping: %w", err)
	}

	return compatHash, nil
}

func (t *Translator) computeBlobCompatHash(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	reader, err := obj.Reader()
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("read object: %w", err)
	}
	defer reader.Close()

	compatHash, err := t.compatHasher.ComputeReader(obj.Type(), obj.Size(), reader)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("compute compat hash: %w", err)
	}
	return compatHash, nil
}

// ImportObject stores an object written in compat format into dst using the
// native object format, then records the native<->compat mapping.
func (t *Translator) ImportObject(obj plumbing.EncodedObject, dst storer.EncodedObjectStorer) (plumbing.Hash, error) {
	content, err := readObjectContent(obj)
	if err != nil {
		return plumbing.Hash{}, err
	}

	nativeContent, err := t.rewriteContentToNative(obj.Type(), content)
	if err != nil {
		return plumbing.Hash{}, err
	}

	nativeHash, err := storeObject(dst, obj.Type(), nativeContent)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("store native object: %w", err)
	}

	if err := t.mapping.Add(nativeHash, obj.Hash()); err != nil {
		return plumbing.Hash{}, fmt.Errorf("record mapping: %w", err)
	}

	return nativeHash, nil
}

func readObjectContent(obj plumbing.EncodedObject) ([]byte, error) {
	reader, err := obj.Reader()
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read object content: %w", err)
	}

	return content, nil
}

func storeObject(dst storer.EncodedObjectStorer, objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	obj := dst.NewEncodedObject()
	obj.SetType(objType)
	obj.SetSize(int64(len(content)))

	w, err := obj.Writer()
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("open object writer: %w", err)
	}

	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.Hash{}, fmt.Errorf("write object content: %w", err)
	}
	if err := w.Close(); err != nil {
		return plumbing.Hash{}, fmt.Errorf("close object writer: %w", err)
	}

	return dst.SetEncodedObject(obj)
}

func (t *Translator) rewriteContentToCompat(objType plumbing.ObjectType, content []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return content, nil
	case plumbing.TreeObject:
		compatContent, err := t.rewriteTreeContent(content, false)
		if err != nil {
			return nil, fmt.Errorf("translate tree: %w", err)
		}
		return compatContent, nil
	case plumbing.CommitObject:
		compatContent, err := t.rewriteCommitContent(content, false)
		if err != nil {
			return nil, fmt.Errorf("translate commit: %w", err)
		}
		return compatContent, nil
	case plumbing.TagObject:
		compatContent, err := t.rewriteTagContent(content, false)
		if err != nil {
			return nil, fmt.Errorf("translate tag: %w", err)
		}
		return compatContent, nil
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

func (t *Translator) rewriteContentToNative(objType plumbing.ObjectType, content []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return content, nil
	case plumbing.TreeObject:
		nativeContent, err := t.rewriteTreeContent(content, true)
		if err != nil {
			return nil, fmt.Errorf("translate tree: %w", err)
		}
		return nativeContent, nil
	case plumbing.CommitObject:
		nativeContent, err := t.rewriteCommitContent(content, true)
		if err != nil {
			return nil, fmt.Errorf("translate commit: %w", err)
		}
		return nativeContent, nil
	case plumbing.TagObject:
		nativeContent, err := t.rewriteTagContent(content, true)
		if err != nil {
			return nil, fmt.Errorf("translate tag: %w", err)
		}
		return nativeContent, nil
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

// rewriteTreeContent rewrites binary hashes in tree entries between native
// and compat formats. Tree entry format: <mode-octal> <name>\0<binary-hash>
func (t *Translator) rewriteTreeContent(content []byte, reverse bool) ([]byte, error) {
	fromSize := t.formats.Native.Size()
	toSize := t.formats.Compat.Size()
	lookup := t.mapping.NativeToCompat
	missingFormat := "compat"
	if reverse {
		fromSize = t.formats.Compat.Size()
		toSize = t.formats.Native.Size()
		lookup = t.mapping.CompatToNative
		missingFormat = "native"
	}

	var out bytes.Buffer
	buf := content

	for len(buf) > 0 {
		// Find the null byte separating "mode name" from the binary hash.
		nullIdx := bytes.IndexByte(buf, 0)
		if nullIdx < 0 {
			return nil, fmt.Errorf("malformed tree entry: missing null byte")
		}

		// Copy everything up to and including the null byte.
		out.Write(buf[:nullIdx+1])
		buf = buf[nullIdx+1:]

		if len(buf) < fromSize {
			return nil, fmt.Errorf("malformed tree entry: truncated hash (have %d, want %d)", len(buf), fromSize)
		}

		fromHash, _ := plumbing.FromBytes(buf[:fromSize])
		buf = buf[fromSize:]

		toHash, err := lookup(fromHash)
		if err != nil {
			return nil, fmt.Errorf("tree entry hash %s: no %s mapping: %w", fromHash, missingFormat, errors.Join(ErrMissingDependencyMapping, err))
		}

		out.Write(toHash.Bytes()[:toSize])
	}

	return out.Bytes(), nil
}

// rewriteCommitContent rewrites commit header hashes between native and compat
// formats, including tree, parent, and embedded mergetag references.
func (t *Translator) rewriteCommitContent(content []byte, reverse bool) ([]byte, error) {
	return t.rewriteCommitTextObject(content, reverse)
}

// rewriteTagContent rewrites the object hash in an annotated tag between
// native and compat formats.
func (t *Translator) rewriteTagContent(content []byte, reverse bool) ([]byte, error) {
	return t.rewriteTextObject(content, []string{"object"}, reverse)
}

// reverseTranslateCommit rewrites compat-format hex hashes on "tree" and
// "parent" lines back to native-format hashes.
func (t *Translator) reverseTranslateCommit(content []byte) ([]byte, error) {
	return t.rewriteCommitContent(content, true)
}

// reverseTranslateTag rewrites the compat-format hex hash on the "object" line
// back to the native object hash.
func (t *Translator) reverseTranslateTag(content []byte) ([]byte, error) {
	return t.rewriteTagContent(content, true)
}

// rewriteTextObject rewrites hex hashes on specified header lines between
// native and compat formats. It processes lines until it hits an empty line.
func (t *Translator) rewriteTextObject(content []byte, hashFields []string, reverse bool) ([]byte, error) {
	fromHexSize := t.formats.Native.HexSize()
	toHexSize := t.formats.Compat.HexSize()
	lookup := t.mapping.NativeToCompat
	missingFormat := "compat"
	if reverse {
		fromHexSize = t.formats.Compat.HexSize()
		toHexSize = t.formats.Native.HexSize()
		lookup = t.mapping.CompatToNative
		missingFormat = "native"
	}

	var out bytes.Buffer
	remaining := content
	headerDone := false

	for len(remaining) > 0 {
		nlIdx := bytes.IndexByte(remaining, '\n')
		var line []byte
		if nlIdx >= 0 {
			line = remaining[:nlIdx]
			remaining = remaining[nlIdx+1:]
		} else {
			line = remaining
			remaining = nil
		}

		if !headerDone {
			if len(line) == 0 {
				// Empty line = end of header.
				headerDone = true
				out.WriteByte('\n')
				continue
			}

			replaced := false
			for _, field := range hashFields {
				prefix := field + " "
				if bytes.HasPrefix(line, []byte(prefix)) && len(line) == len(prefix)+fromHexSize {
					hexStr := string(line[len(prefix):])
					fromHash, ok := plumbing.FromHex(hexStr)
					if !ok {
						return nil, fmt.Errorf("invalid hash on %s line: %q", field, hexStr)
					}

					toHash, err := lookup(fromHash)
					if err != nil {
						return nil, fmt.Errorf("%s hash %s: no %s mapping: %w", field, fromHash, missingFormat, errors.Join(ErrMissingDependencyMapping, err))
					}

					out.WriteString(prefix)
					out.WriteString(toHash.String()[:toHexSize])
					out.WriteByte('\n')
					replaced = true
					break
				}
			}

			if !replaced {
				out.Write(line)
				out.WriteByte('\n')
			}
		} else {
			// Body: copy verbatim.
			out.Write(line)
			if nlIdx >= 0 {
				out.WriteByte('\n')
			}
		}
	}

	return out.Bytes(), nil
}

func (t *Translator) rewriteCommitTextObject(content []byte, reverse bool) ([]byte, error) {
	var out bytes.Buffer
	remaining := content
	headerDone := false
	var pending *lineInfo

	for pending != nil || len(remaining) > 0 {
		li := pending
		if li != nil {
			pending = nil
		} else {
			next := nextLineInfo(remaining)
			li = &next
			remaining = next.rest
		}

		line := li.line

		if !headerDone {
			if len(line) == 0 {
				headerDone = true
				if li.hasNewline {
					out.WriteByte('\n')
				}
				continue
			}

			if bytes.HasPrefix(line, []byte("mergetag ")) {
				nextPending, err := t.translateMergeTagSection(&out, line, remaining, reverse)
				if err != nil {
					return nil, err
				}
				if nextPending != nil {
					remaining = nextPending.rest
					pending = nextPending
				} else {
					remaining = nil
				}
				continue
			}

			fields := []string{"tree", "parent"}
			translated, err := t.rewriteHeaderHashLine(line, fields, reverse)
			if err != nil {
				return nil, err
			}
			out.Write(translated)
			if li.hasNewline {
				out.WriteByte('\n')
			}
			continue
		}

		out.Write(line)
		if li.hasNewline {
			out.WriteByte('\n')
		}
	}

	return out.Bytes(), nil
}

func (t *Translator) rewriteHeaderHashLine(line []byte, hashFields []string, reverse bool) ([]byte, error) {
	fromHexSize := t.formats.Native.HexSize()
	lookup := t.mapping.NativeToCompat
	missingFormat := "compat"
	if reverse {
		fromHexSize = t.formats.Compat.HexSize()
		lookup = t.mapping.CompatToNative
		missingFormat = "native"
	}

	for _, field := range hashFields {
		prefix := field + " "
		if bytes.HasPrefix(line, []byte(prefix)) && len(line) == len(prefix)+fromHexSize {
			fromHash, ok := plumbing.FromHex(string(line[len(prefix):]))
			if !ok {
				return nil, fmt.Errorf("invalid hash on %s line: %q", field, string(line[len(prefix):]))
			}

			toHash, err := lookup(fromHash)
			if err != nil {
				return nil, fmt.Errorf("%s hash %s: no %s mapping: %w", field, fromHash, missingFormat, errors.Join(ErrMissingDependencyMapping, err))
			}

			return []byte(prefix + toHash.String()), nil
		}
	}

	return line, nil
}

type lineInfo struct {
	line       []byte
	rest       []byte
	hasNewline bool
}

func nextLineInfo(content []byte) lineInfo {
	nlIdx := bytes.IndexByte(content, '\n')
	if nlIdx >= 0 {
		return lineInfo{
			line:       content[:nlIdx],
			rest:       content[nlIdx+1:],
			hasNewline: true,
		}
	}

	return lineInfo{
		line: content,
	}
}

func (t *Translator) translateMergeTagSection(out *bytes.Buffer, firstLine, remaining []byte, reverse bool) (*lineInfo, error) {
	payloadLines := [][]byte{firstLine[len("mergetag "):]}
	var nextPending *lineInfo

	for len(remaining) > 0 {
		next := nextLineInfo(remaining)
		remaining = next.rest
		if len(next.line) > 0 && next.line[0] == ' ' {
			payloadLines = append(payloadLines, next.line[1:])
			continue
		}
		nextPending = &next
		break
	}

	payload := bytes.Join(payloadLines, []byte("\n"))
	translated, err := t.rewriteTagContent(payload, reverse)
	if err != nil {
		return nil, fmt.Errorf("translate mergetag: %w", err)
	}

	lines := bytes.Split(translated, []byte("\n"))
	out.WriteString("mergetag ")
	out.Write(lines[0])
	for _, line := range lines[1:] {
		out.WriteByte('\n')
		out.WriteByte(' ')
		out.Write(line)
	}
	if nextPending != nil || len(remaining) > 0 {
		out.WriteByte('\n')
	}

	return nextPending, nil
}

// ReverseTranslateContent takes object content in native format and returns
// it in compat format. This is the inverse of what TranslateObject does
// internally -- it rewrites hash references from native to compat format.
//
// This is needed for push operations where objects must be sent in the
// compat format to a server that uses that format.
func (t *Translator) ReverseTranslateContent(objType plumbing.ObjectType, nativeContent []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return nativeContent, nil
	case plumbing.TreeObject:
		return t.rewriteTreeContent(nativeContent, false)
	case plumbing.CommitObject:
		return t.rewriteCommitContent(nativeContent, false)
	case plumbing.TagObject:
		return t.rewriteTagContent(nativeContent, false)
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

// ComputeNativeHash computes the native-format hash for raw content.
// This is a convenience method for callers that need to hash content
// that is already in native format.
func (t *Translator) ComputeNativeHash(objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	return t.nativeHasher.Compute(objType, content)
}

// ComputeCompatHash computes the compat-format hash for raw content.
func (t *Translator) ComputeCompatHash(objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	return t.compatHasher.Compute(objType, content)
}

// NativeObjectFormat returns the native object format.
func (t *Translator) NativeObjectFormat() format.ObjectFormat {
	return t.formats.Native
}

// CompatObjectFormat returns the compat object format.
func (t *Translator) CompatObjectFormat() format.ObjectFormat {
	return t.formats.Compat
}
