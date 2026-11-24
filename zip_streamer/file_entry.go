package zip_streamer

import (
	"errors"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type FileEntry struct {
	url     *url.URL
	zipPath string

	// Optional metadata. When size and crc32 are known for every file in a
	// zip, the exact archive layout can be computed up front, which enables
	// Content-Length and HTTP Range requests.
	size     int64
	crc32    uint32
	hasSize  bool
	hasCRC32 bool
	modified time.Time
}

const UrlPrefixEnvVar = "ZS_URL_PREFIX"

func NewFileEntry(urlString string, zipPath string) (*FileEntry, error) {
	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	if url.Scheme != "http" && url.Scheme != "https" {
		return nil, errors.New("url must be a http url")
	}
	urlPrefix := os.Getenv(UrlPrefixEnvVar)
	if !strings.HasPrefix(urlString, urlPrefix) {
		return nil, errors.New("URL not allowed")
	}

	zipPath = path.Clean(zipPath)
	if path.IsAbs(zipPath) {
		return nil, errors.New("zip path must be relative")
	}
	if filename := path.Base(zipPath); len(filename) == 0 || filename == "." {
		return nil, errors.New("zip path must have file")
	}

	f := FileEntry{
		url:     url,
		zipPath: zipPath,
	}
	return &f, nil
}

func (f *FileEntry) Url() *url.URL {
	return f.url
}

func (f *FileEntry) ZipPath() string {
	return f.zipPath
}

// SetSize sets the exact size of the file in bytes.
func (f *FileEntry) SetSize(size int64) error {
	if size < 0 {
		return errors.New("size must not be negative")
	}
	f.size = size
	f.hasSize = true
	return nil
}

// SetCRC32 sets the CRC-32 (IEEE) checksum of the file contents.
func (f *FileEntry) SetCRC32(crc uint32) {
	f.crc32 = crc
	f.hasCRC32 = true
}

// SetModified sets the modification time stored in the zip entry.
func (f *FileEntry) SetModified(t time.Time) {
	f.modified = t
}

// Size returns the file size and whether it is known.
func (f *FileEntry) Size() (int64, bool) {
	return f.size, f.hasSize
}

// CRC32 returns the CRC-32 checksum and whether it is known.
func (f *FileEntry) CRC32() (uint32, bool) {
	return f.crc32, f.hasCRC32
}

// Modified returns the modification time, zero if not set.
func (f *FileEntry) Modified() time.Time {
	return f.modified
}

// HasSizeAndCRC32 reports whether both size and CRC-32 are known.
func (f *FileEntry) HasSizeAndCRC32() bool {
	return f.hasSize && f.hasCRC32
}
