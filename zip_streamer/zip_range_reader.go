package zip_streamer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// ZipLayoutReader is an io.ReadSeeker over the bytes of a ZipLayout. Static
// parts (headers, central directory) are served from memory; file contents
// are fetched lazily from their URLs. When reading starts in the middle of a
// file, an upstream HTTP Range request is used (falling back to skipping
// bytes if the upstream server ignores ranges).
//
// It is designed to be used with http.ServeContent, which implements
// Range, If-Range, HEAD and multipart range responses on top of it.
type ZipLayoutReader struct {
	layout *ZipLayout
	ctx    context.Context
	client *http.Client

	mu  sync.Mutex
	pos int64
	err error // first read error, if any

	// currently open upstream body
	body    io.ReadCloser
	bodySeg int   // segment index of body
	bodyPos int64 // archive position the body will produce next
}

// NewZipLayoutReader creates a reader. ctx cancels upstream requests.
func NewZipLayoutReader(ctx context.Context, layout *ZipLayout) *ZipLayoutReader {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ZipLayoutReader{
		layout:  layout,
		ctx:     ctx,
		client:  http.DefaultClient,
		bodySeg: -1,
	}
}

func (r *ZipLayoutReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = r.layout.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	r.pos = abs
	return abs, nil
}

func (r *ZipLayoutReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.read(p)
	if err != nil && err != io.EOF && r.err == nil {
		r.err = err
	}
	return n, err
}

// Err returns the first error encountered while reading (excluding io.EOF).
func (r *ZipLayoutReader) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *ZipLayoutReader) read(p []byte) (int, error) {
	if r.pos >= r.layout.size {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}

	idx := r.layout.segmentIndex(r.pos)
	seg := r.layout.segments[idx]
	inSeg := r.pos - seg.offset
	remaining := seg.size - inSeg
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	if seg.data != nil {
		n := copy(p, seg.data[inSeg:])
		r.pos += int64(n)
		return n, nil
	}

	if r.body == nil || r.bodySeg != idx || r.bodyPos != r.pos {
		r.closeBody()
		body, err := r.openUpstream(seg.entry, inSeg, seg.size)
		if err != nil {
			return 0, err
		}
		r.body, r.bodySeg, r.bodyPos = body, idx, r.pos
	}

	n, err := r.body.Read(p)
	r.pos += int64(n)
	r.bodyPos += int64(n)
	if err == io.EOF {
		r.closeBody()
		if r.pos == seg.offset+seg.size {
			err = nil
		} else {
			err = fmt.Errorf("%s: upstream file shorter than declared size", seg.entry.ZipPath())
		}
	}
	return n, err
}

// Close releases the open upstream connection, if any.
func (r *ZipLayoutReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeBody()
	return nil
}

func (r *ZipLayoutReader) closeBody() {
	if r.body != nil {
		r.body.Close()
		r.body = nil
		r.bodySeg = -1
	}
}

// openUpstream returns a body producing the file's bytes from start up to
// its declared size.
func (r *ZipLayoutReader) openUpstream(entry *FileEntry, start int64, size int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, entry.Url().String(), nil)
	if err != nil {
		return nil, err
	}
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, size-1))
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}

	fail := func(format string, args ...any) (io.ReadCloser, error) {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: "+format, append([]any{entry.ZipPath()}, args...)...)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		if resp.ContentLength >= 0 && resp.ContentLength != size {
			return fail("upstream size %d does not match declared size %d", resp.ContentLength, size)
		}
		if start > 0 {
			// Upstream ignored the Range header: skip to the wanted offset.
			if _, err := io.CopyN(io.Discard, resp.Body, start); err != nil {
				return fail("skipping to offset %d: %v", start, err)
			}
		}
	case http.StatusPartialContent:
		rangeStart, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || rangeStart != start || (total >= 0 && total != size) {
			return fail("unexpected Content-Range %q", resp.Header.Get("Content-Range"))
		}
	default:
		return fail("upstream returned status %d", resp.StatusCode)
	}

	return &limitedReadCloser{Reader: io.LimitReader(resp.Body, size-start), Closer: resp.Body}, nil
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}

// parseContentRange parses "bytes start-end/total". total is -1 for "*".
func parseContentRange(v string) (start int64, total int64, ok bool) {
	v, found := strings.CutPrefix(strings.TrimSpace(v), "bytes ")
	if !found {
		return 0, 0, false
	}
	rng, totalStr, found := strings.Cut(v, "/")
	if !found {
		return 0, 0, false
	}
	startStr, _, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startStr), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	total = -1
	if totalStr = strings.TrimSpace(totalStr); totalStr != "*" {
		total, err = strconv.ParseInt(totalStr, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	return start, total, true
}
