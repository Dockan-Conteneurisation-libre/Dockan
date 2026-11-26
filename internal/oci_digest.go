package internal

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
	"strings"
)

// maxOCIMetadataBytes caps manifests and image configs read into memory.
const maxOCIMetadataBytes = 8 << 20

var ociDigestPattern = regexp.MustCompile(`^(sha256:[a-f0-9]{64}|sha512:[a-f0-9]{128})$`)

// validateOCIDigest rejects malformed digests before they are used in a URL
// or compared against downloaded content.
func validateOCIDigest(digest string) error {
	if !ociDigestPattern.MatchString(digest) {
		return fmt.Errorf("digest OCI invalide: %q", digest)
	}
	return nil
}

func newOCIDigestHash(digest string) (hash.Hash, error) {
	if err := validateOCIDigest(digest); err != nil {
		return nil, err
	}
	if strings.HasPrefix(digest, "sha512:") {
		return sha512.New(), nil
	}
	return sha256.New(), nil
}

func checkOCIDigest(digest string, h hash.Hash) error {
	algo, _, _ := strings.Cut(digest, ":")
	got := algo + ":" + hex.EncodeToString(h.Sum(nil))
	if got != digest {
		return fmt.Errorf("empreinte invalide: attendu %s, reçu %s", digest, got)
	}
	return nil
}

// readOCIMetadata reads a bounded metadata document (manifest or config).
func readOCIMetadata(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxOCIMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxOCIMetadataBytes {
		return nil, fmt.Errorf("document OCI trop volumineux (> %d octets)", maxOCIMetadataBytes)
	}
	return body, nil
}

// readVerifiedOCIMetadata reads a document addressed by digest and checks
// that its content matches that digest.
func readVerifiedOCIMetadata(r io.Reader, digest string) ([]byte, error) {
	body, err := readOCIMetadata(r)
	if err != nil {
		return nil, err
	}
	if err := verifyOCIMetadata(body, digest); err != nil {
		return nil, err
	}
	return body, nil
}

func verifyOCIMetadata(body []byte, digest string) error {
	h, err := newOCIDigestHash(digest)
	if err != nil {
		return err
	}
	h.Write(body)
	return checkOCIDigest(digest, h)
}

// maxOCILayerBytes caps a single compressed layer spooled to disk.
const maxOCILayerBytes int64 = 32 << 30

// validateOCILayerSize requires the size every OCI descriptor must carry and
// bounds it, so a registry cannot make Dockan fill the disk.
func validateOCILayerSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("taille de couche absente ou invalide: %d", size)
	}
	if size > maxOCILayerBytes {
		return fmt.Errorf("couche trop volumineuse: %d octets (maximum %d)", size, maxOCILayerBytes)
	}
	return nil
}

// spoolVerifiedBlob copies a layer blob into a temporary file inside workDir
// and verifies its size and digest before anything is extracted from it.
// The caller closes and removes the returned file.
func spoolVerifiedBlob(r io.Reader, digest string, size int64, workDir string) (*os.File, error) {
	h, err := newOCIDigestHash(digest)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(workDir, ".layer-*.blob")
	if err != nil {
		return nil, err
	}
	src := r
	if size > 0 {
		src = io.LimitReader(r, size+1)
	}
	written, err := io.Copy(io.MultiWriter(tmp, h), src)
	if err == nil && size > 0 && written != size {
		err = fmt.Errorf("taille de couche invalide: attendu %d octets, reçu %d", size, written)
	}
	if err == nil {
		err = checkOCIDigest(digest, h)
	}
	if err == nil {
		_, err = tmp.Seek(0, io.SeekStart)
	}
	if err != nil {
		discardTempFile(tmp)
		return nil, err
	}
	return tmp, nil
}

func discardTempFile(f *os.File) {
	name := f.Name()
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[dockan] fermeture de %s: %v\n", name, err)
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "[dockan] suppression de %s: %v\n", name, err)
	}
}

// openLayerStream returns the tar stream of a layer, decompressing it when
// it starts with the gzip magic bytes.
func openLayerStream(r io.Reader) (io.Reader, func() error, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return nil, nil, err
	}
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, nil, err
		}
		return gz, gz.Close, nil
	}
	return br, func() error { return nil }, nil
}
