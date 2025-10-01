package zip_streamer

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

type ReaderFunc func() (io.ReadCloser, error)

type FileEntry struct {
	reader  ReaderFunc
	zipPath string
}

func SimpleGet(url string) ReaderFunc {
	return func() (io.ReadCloser, error) {
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
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
		reader:  SimpleGet(urlString),
		zipPath: zipPath,
	}
	return &f, nil
}

func (f *FileEntry) ZipPath() string {
	return f.zipPath
}
