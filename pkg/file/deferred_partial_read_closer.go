package file

import (
	"errors"
	"io"
	"os"
)

var _ io.ReadCloser = (*deferredPartialReadCloser)(nil)

// deferredPartialReadCloser is a "lazy" read closer, allocating a file descriptor for the given path only upon the first Read() call.
// Additionally only part of the file is allowed to be read, starting at a given position.
type deferredPartialReadCloser struct {
	// path is the path to be opened
	path string
	// file is the io.ReadCloser source for the path
	file   *os.File
	reader io.Reader
	start  int64
	size   int64
}

// NewDeferredPartialReadCloser creates a new NewDeferredPartialReadCloser for the given path.
func newDeferredPartialReadCloser(path string, start, size int64) *deferredPartialReadCloser {
	return &deferredPartialReadCloser{
		path:  path,
		start: start,
		size:  size,
	}
}

// Read implements the io.Reader interface for the previously loaded path, opening the file upon the first invocation.
func (d *deferredPartialReadCloser) Read(b []byte) (int, error) {
	if d.reader == nil {
		file, err := os.Open(d.path)
		if err != nil {
			return 0, err
		}

		_, err = file.Seek(d.start, io.SeekStart)
		if err != nil {
			return 0, err
		}

		d.file = file
		d.reader = io.LimitReader(d.file, d.size)
	}
	n, err := d.reader.Read(b)
	if err != nil && errors.Is(err, io.EOF) {
		// we've reached the end of the file, force a release of the file descriptor. If the file has already been
		// closed, ignore the error.
		if closeErr := d.file.Close(); !errors.Is(closeErr, os.ErrClosed) {
			err = closeErr
		}
	}
	return n, err
}

// Close implements the io.Closer interface for the previously loaded path / opened file.
func (d *deferredPartialReadCloser) Close() error {
	if d.file == nil {
		return nil
	}

	err := d.file.Close()
	if err != nil && errors.Is(err, os.ErrClosed) {
		// ignore the fact that this file has already been closed
		err = nil
	}
	d.file = nil
	d.reader = nil
	return err
}
