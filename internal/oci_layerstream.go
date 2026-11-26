package internal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

var (
	gzipMagic = []byte{0x1f, 0x8b}
	zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// maxDecodedLayerBytes caps the uncompressed size of one layer, so a small
// compressed blob cannot fill the disk (decompression bomb).
var maxDecodedLayerBytes int64 = 64 << 30

// maxTrailingBytes caps what is drained after the end of the tar archive.
const maxTrailingBytes = 1 << 20

var errLayerTooLarge = errors.New("couche décompressée trop volumineuse")

// cappedReader fails instead of silently truncating when more than limit
// bytes are read.
type cappedReader struct {
	r     io.Reader
	limit int64
	read  int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	if c.read > c.limit {
		return n, errLayerTooLarge
	}
	return n, err
}

// zstdCommand is the decompressor used for zstd layers. Go has no zstd
// decoder in its standard library and Dockan has no external dependency, so
// the host's zstd tool does the work.
var zstdCommand = "zstd"

// layerStream is the uncompressed tar stream of a layer.
type layerStream struct {
	io.Reader
	// finish releases the stream after a full read and reports decoding
	// errors; abort releases it after a failed extraction.
	finish func() error
	abort  func()
}

// openLayerStream detects the layer compression from its first bytes: gzip
// (decoded in Go), zstd (decoded by the host zstd tool) or plain tar.
func openLayerStream(r io.Reader) (*layerStream, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(len(zstdMagic))
	if err != nil && err != io.EOF {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(magic, gzipMagic):
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, err
		}
		return &layerStream{Reader: gz, finish: gz.Close, abort: func() { gz.Close() }}, nil
	case bytes.HasPrefix(magic, zstdMagic):
		return openZstdStream(br)
	default:
		return &layerStream{Reader: br, finish: func() error { return nil }, abort: func() {}}, nil
	}
}

func openZstdStream(r io.Reader) (*layerStream, error) {
	path, err := exec.LookPath(zstdCommand)
	if err != nil {
		return nil, fmt.Errorf("cette image utilise des couches zstd: installez l'outil zstd " +
			"(par exemple « dnf install zstd » ou « apt install zstd »), puis relancez")
	}
	cmd := exec.Command(path, "-d", "-c", "-q")
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lancement de zstd: %w", err)
	}
	abort := func() {
		_ = cmd.Process.Kill() // extraction already failed; the process only needs to end
		_ = cmd.Wait()
	}
	finish := func() error {
		// Drain what tar did not read (end-of-archive padding) so zstd can
		// exit cleanly, then check its status. More than a little padding
		// means the stream carries data outside the archive.
		n, copyErr := io.Copy(io.Discard, io.LimitReader(out, maxTrailingBytes+1))
		if n > maxTrailingBytes {
			abort()
			return errors.New("données inattendues après la fin de l'archive de la couche")
		}
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("décompression zstd: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		return copyErr
	}
	return &layerStream{Reader: out, finish: finish, abort: abort}, nil
}

// extractLayerBlob decompresses a verified layer blob and applies it to
// destDir.
func extractLayerBlob(blob io.Reader, destDir string) error {
	stream, err := openLayerStream(blob)
	if err != nil {
		return err
	}
	capped := &cappedReader{r: stream.Reader, limit: maxDecodedLayerBytes}
	if err := extractTarStream(capped, destDir); err != nil {
		stream.abort()
		return err
	}
	if err := stream.finish(); err != nil {
		return errors.Join(errors.New("couche corrompue"), err)
	}
	return nil
}
