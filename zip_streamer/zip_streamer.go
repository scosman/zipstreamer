package zip_streamer

import (
	"archive/zip"
	"errors"
	"io"
	"net/http"
	"time"
)

type ZipStream struct {
	entries           []*FileEntry
	destination       io.Writer
	CompressionMethod uint16
}

func NewZipStream(entries []*FileEntry, w io.Writer) (*ZipStream, error) {
	if len(entries) == 0 {
		return nil, errors.New("must have at least 1 entry")
	}

	z := ZipStream{
		entries:     entries,
		destination: w,
		// Default to no compression to save CPU. Also ideal for streaming.
		CompressionMethod: zip.Store,
	}

	return &z, nil
}

func (z *ZipStream) StreamAllFiles(maxUpstreamRetries int) error {
	zipWriter := zip.NewWriter(z.destination)
	success := 0

	for _, entry := range z.entries {
		resp, err := retryableGet(entry.Url().String(), maxUpstreamRetries)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}

		header := &zip.FileHeader{
			Name:     entry.ZipPath(),
			Method:   z.CompressionMethod,
			Modified: time.Now(),
		}
		entryWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		_, err = io.Copy(entryWriter, resp.Body)
		if err != nil {
			return err
		}

		zipWriter.Flush()
		flushingWriter, ok := z.destination.(http.Flusher)
		if ok {
			flushingWriter.Flush()
		}

		success++
	}

	if success == 0 {
		return errors.New("empty file - all files failed")
	}

	return zipWriter.Close()
}

func retryableGet(url string, maxRetries int) (*http.Response, error) {
	var err error

	for i := 0; i < maxRetries+1; i++ {
		resp, err := http.Get(url)
		if err != nil {
			// TODO: exponential backoff?
			time.Sleep(1)
			continue
		} else {
			return resp, nil
		}
	}

	return nil, err
}
