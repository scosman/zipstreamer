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

func (z *ZipStream) StreamAllFiles() error {
	zipWriter := zip.NewWriter(z.destination)
	success := 0

	for _, entry := range z.entries {
		resp, err := http.Get(entry.Url().String())
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

		// flush after every 32kb instead of every file to reduce memory
		for {
			_, err := io.CopyN(entryWriter, resp.Body, 32<<10)
			if err != nil {

				// Files chunks that are less than 32KB should be flushed.
				// Chunks of the file has been copied, so we can flush.
				zipWriter.Flush()
				flushingWriter, ok := z.destination.(http.Flusher)
				if ok {
					flushingWriter.Flush()
				}

				if errors.Is(err, io.EOF) {

					// We have completed copying the file.
					break
				}

				return err
			}

			// Files chunks that do not fit in 32KB should be flushed.
			// Chunks of the file has been copied, so we can flush.
			zipWriter.Flush()
			flushingWriter, ok := z.destination.(http.Flusher)
			if ok {
				flushingWriter.Flush()
			}
		}

		success++
	}

	if success == 0 {
		return errors.New("empty file - all files failed")
	}

	return zipWriter.Close()
}
