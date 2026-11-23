package testing

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
)

// upstream serves test files. If ignoreRanges is set, Range headers are
// ignored and the full file is always returned with status 200.
type upstream struct {
	files        map[string][]byte
	ignoreRanges bool
	hits         atomic.Int64
	server       *httptest.Server
}

func newUpstream(t *testing.T, files map[string][]byte) *upstream {
	u := &upstream{files: files}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		data, ok := u.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if u.ignoreRanges {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Write(data)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(u.server.Close)
	return u
}

type testFile struct {
	path    string
	zipPath string
	data    []byte
}

func testFiles() []testFile {
	big := make([]byte, 300000)
	for i := range big {
		big[i] = byte(i * 7 % 251)
	}
	return []testFile{
		{"/a.txt", "a.txt", []byte("hello world\n")},
		{"/empty.txt", "empty.txt", []byte{}},
		{"/big.bin", "folder/big.bin", big},
		{"/umlaut.txt", "ordner/Grüße.txt", []byte("Grüße aus dem Test\n")},
	}
}

func sizedDescriptor(u *upstream, files []testFile, crcAsNumber bool) []byte {
	type entry struct {
		Url      string `json:"url"`
		ZipPath  string `json:"zipPath"`
		Size     int    `json:"size"`
		CRC32    any    `json:"crc32"`
		Modified string `json:"modified,omitempty"`
	}
	entries := []entry{}
	for _, f := range files {
		var crc any = fmt.Sprintf("%08x", crc32.ChecksumIEEE(f.data))
		if crcAsNumber {
			crc = crc32.ChecksumIEEE(f.data)
		}
		entries = append(entries, entry{
			Url:      u.server.URL + f.path,
			ZipPath:  f.zipPath,
			Size:     len(f.data),
			CRC32:    crc,
			Modified: "2024-05-06T07:08:10Z",
		})
	}
	body, _ := json.Marshal(map[string]any{"suggestedFilename": "download.zip", "files": entries})
	return body
}

func setupSized(t *testing.T) (*upstream, []testFile, []byte) {
	files := testFiles()
	m := map[string][]byte{}
	for _, f := range files {
		m[f.path] = f.data
	}
	u := newUpstream(t, m)
	return u, files, sizedDescriptor(u, files, false)
}

// doRequest sends a download request for the descriptor. POST uses
// POST /download, GET and HEAD use a link from /create_download_link.
func doRequest(t *testing.T, method string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	server := zip_streamer.NewServer()
	path := "/download"
	if method != "POST" {
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, httptest.NewRequest("POST", "/create_download_link", bytes.NewReader(body)))
		var resp struct {
			LinkId string `json:"link_id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		path = "/download_link/" + resp.LinkId
		body = nil
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	return rr
}

func verifyZip(t *testing.T, zipBytes []byte, files []testFile) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	if len(zr.File) != len(files) {
		t.Fatalf("expected %d files, got %d", len(files), len(zr.File))
	}
	for i, f := range zr.File {
		if f.Name != files[i].zipPath {
			t.Errorf("file %d: name %q, want %q", i, f.Name, files[i].zipPath)
		}
		if !f.Modified.Equal(time.Date(2024, 5, 6, 7, 8, 10, 0, time.UTC)) {
			t.Errorf("file %d: modified %v", i, f.Modified)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc) // also verifies CRC-32
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		if !bytes.Equal(data, files[i].data) {
			t.Errorf("file %s: content mismatch", f.Name)
		}
	}
}

func TestSizedDownloadHasContentLength(t *testing.T) {
	_, files, desc := setupSized(t)
	rr := doRequest(t, "POST", desc, nil)
	checkHttpOk(rr, t)

	res := rr.Result()
	body := rr.Body.Bytes()
	if res.Header.Get("Content-Length") != fmt.Sprint(len(body)) {
		t.Fatalf("Content-Length %q, body %d", res.Header.Get("Content-Length"), len(body))
	}
	if res.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q", res.Header.Get("Accept-Ranges"))
	}
	if res.Header.Get("ETag") == "" {
		t.Errorf("missing ETag")
	}
	if res.Header.Get("Content-Type") != "application/zip" {
		t.Errorf("Content-Type = %q", res.Header.Get("Content-Type"))
	}
	if res.Header.Get("Content-Disposition") != `attachment; filename="download.zip"` {
		t.Errorf("Content-Disposition = %q", res.Header.Get("Content-Disposition"))
	}
	verifyZip(t, body, files)

	layout, err := zip_streamer.NewZipLayout(mustParse(t, desc).Files())
	if err != nil {
		t.Fatal(err)
	}
	if layout.Size() != int64(len(body)) {
		t.Errorf("layout size %d != body %d", layout.Size(), len(body))
	}
}

func TestSizedPostIgnoresRange(t *testing.T) {
	// Range handling is only defined for GET (RFC 9110)
	_, files, desc := setupSized(t)
	rr := doRequest(t, "POST", desc, map[string]string{"Range": "bytes=0-9"})
	checkHttpOk(rr, t)
	verifyZip(t, rr.Body.Bytes(), files)
}

func TestSizedDownloadNumericCRC(t *testing.T) {
	u, files, _ := setupSized(t)
	rr := doRequest(t, "POST", sizedDescriptor(u, files, true), nil)
	checkHttpOk(rr, t)
	verifyZip(t, rr.Body.Bytes(), files)
}

func mustParse(t *testing.T, desc []byte) *zip_streamer.ZipDescriptor {
	zd, err := zip_streamer.UnmarshalJsonZipDescriptor(desc)
	if err != nil {
		t.Fatal(err)
	}
	return zd
}

func testRanges(t *testing.T, ignoreRanges bool) {
	u, _, desc := setupSized(t)
	u.ignoreRanges = ignoreRanges
	full := doRequest(t, "GET", desc, nil).Body.Bytes()
	size := len(full)

	ranges := [][2]int{
		{0, 0}, {0, 29}, {10, 100}, {29, 60}, {50, 50000}, {1000, size - 1},
		{size - 22, size - 1}, {size - 1, size - 1}, {100000, 250000},
	}
	for _, r := range ranges {
		rr := doRequest(t, "GET", desc, map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", r[0], r[1])})
		if rr.Code != http.StatusPartialContent {
			t.Fatalf("range %v: status %d", r, rr.Code)
		}
		want := full[r[0] : r[1]+1]
		if !bytes.Equal(rr.Body.Bytes(), want) {
			t.Fatalf("range %v: content mismatch (got %d bytes, want %d)", r, rr.Body.Len(), len(want))
		}
		wantCR := fmt.Sprintf("bytes %d-%d/%d", r[0], r[1], size)
		if cr := rr.Result().Header.Get("Content-Range"); cr != wantCR {
			t.Errorf("range %v: Content-Range %q, want %q", r, cr, wantCR)
		}
	}

	// open ended & suffix ranges
	rr := doRequest(t, "GET", desc, map[string]string{"Range": "bytes=12345-"})
	if !bytes.Equal(rr.Body.Bytes(), full[12345:]) {
		t.Errorf("open ended range mismatch")
	}
	rr = doRequest(t, "GET", desc, map[string]string{"Range": "bytes=-500"})
	if !bytes.Equal(rr.Body.Bytes(), full[size-500:]) {
		t.Errorf("suffix range mismatch")
	}

	// invalid range
	rr = doRequest(t, "GET", desc, map[string]string{"Range": fmt.Sprintf("bytes=%d-", size+10)})
	if rr.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("unsatisfiable range: status %d", rr.Code)
	}
}

func TestSizedDownloadRanges(t *testing.T) {
	testRanges(t, false)
}

func TestSizedDownloadRangesUpstreamIgnoresRanges(t *testing.T) {
	testRanges(t, true)
}

func TestSizedDownloadMultipartRanges(t *testing.T) {
	_, _, desc := setupSized(t)
	full := doRequest(t, "GET", desc, nil).Body.Bytes()

	rr := doRequest(t, "GET", desc, map[string]string{"Range": "bytes=0-99,200000-200099"})
	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status %d", rr.Code)
	}
	_, params, err := mime.ParseMediaType(rr.Result().Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(rr.Body, params["boundary"])
	expected := [][]byte{full[0:100], full[200000:200100]}
	for i, want := range expected {
		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("part %d: %v", i, err)
		}
		got, _ := io.ReadAll(part)
		if !bytes.Equal(got, want) {
			t.Errorf("part %d mismatch", i)
		}
	}
}

func TestSizedDownloadIfRange(t *testing.T) {
	_, _, desc := setupSized(t)
	full := doRequest(t, "GET", desc, nil)
	etag := full.Result().Header.Get("ETag")

	rr := doRequest(t, "GET", desc, map[string]string{"Range": "bytes=100-", "If-Range": etag})
	if rr.Code != http.StatusPartialContent || !bytes.Equal(rr.Body.Bytes(), full.Body.Bytes()[100:]) {
		t.Errorf("If-Range with matching etag: status %d", rr.Code)
	}
	rr = doRequest(t, "GET", desc, map[string]string{"Range": "bytes=100-", "If-Range": `"other"`})
	if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), full.Body.Bytes()) {
		t.Errorf("If-Range with other etag should return full content: status %d", rr.Code)
	}
}

func TestSizedDownloadETagStableAcrossUrls(t *testing.T) {
	u, files, desc := setupSized(t)
	etag1 := doRequest(t, "HEAD", desc, nil).Result().Header.Get("ETag")

	// Same files, but different URLs (e.g. rotated signed URLs)
	desc2 := bytes.ReplaceAll(desc, []byte(u.server.URL+"/"), []byte(u.server.URL+"/?sig=2&x=/"))
	etag2 := doRequest(t, "HEAD", desc2, nil).Result().Header.Get("ETag")
	if etag1 != etag2 {
		t.Errorf("etag changed with URL: %s vs %s", etag1, etag2)
	}

	files[0].zipPath = "renamed.txt"
	etag3 := doRequest(t, "HEAD", sizedDescriptor(u, files, false), nil).Result().Header.Get("ETag")
	if etag1 == etag3 {
		t.Errorf("etag should change when contents change")
	}
}

func TestSizedHeadDoesNotFetchFiles(t *testing.T) {
	u, _, desc := setupSized(t)
	full := doRequest(t, "GET", desc, nil)
	u.hits.Store(0)

	rr := doRequest(t, "HEAD", desc, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if rr.Result().Header.Get("Content-Length") != fmt.Sprint(full.Body.Len()) {
		t.Errorf("HEAD Content-Length %q", rr.Result().Header.Get("Content-Length"))
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD returned body")
	}
	if u.hits.Load() != 0 {
		t.Errorf("HEAD fetched %d upstream files", u.hits.Load())
	}
}

func TestCreateLinkReturnsSizeAndDownloadLinkSupportsRanges(t *testing.T) {
	_, files, desc := setupSized(t)
	server := zip_streamer.NewServer()

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest("POST", "/create_download_link", bytes.NewReader(desc)))
	checkHttpOk(rr, t)
	var resp struct {
		Status string `json:"status"`
		LinkId string `json:"link_id"`
		Size   int64  `json:"size"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest("GET", "/download_link/"+resp.LinkId, nil))
	checkHttpOk(rr, t)
	if int64(rr.Body.Len()) != resp.Size || resp.Size == 0 {
		t.Fatalf("size in link response %d, body %d", resp.Size, rr.Body.Len())
	}
	verifyZip(t, rr.Body.Bytes(), files)

	req := httptest.NewRequest("GET", "/download_link/"+resp.LinkId, nil)
	req.Header.Set("Range", "bytes=10-19")
	rr2 := httptest.NewRecorder()
	server.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusPartialContent || !bytes.Equal(rr2.Body.Bytes(), rr.Body.Bytes()[10:20]) {
		t.Errorf("range on download link failed: %d", rr2.Code)
	}
}

func TestUnsizedDescriptorFallsBackToStreaming(t *testing.T) {
	u, files, _ := setupSized(t)
	desc := []byte(fmt.Sprintf(`{"suggestedFilename":"download.zip","files":[
		{"url":"%s/a.txt","zipPath":"a.txt","size":12,"crc32":"%08x"},
		{"url":"%s/empty.txt","zipPath":"empty.txt"}]}`,
		u.server.URL, crc32.ChecksumIEEE(files[0].data), u.server.URL))
	rr := doRequest(t, "GET", desc, map[string]string{"Range": "bytes=0-9"})
	checkHttpOk(rr, t)
	if rr.Result().Header.Get("Accept-Ranges") != "none" {
		t.Errorf("Accept-Ranges = %q", rr.Result().Header.Get("Accept-Ranges"))
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil || len(zr.File) != 2 {
		t.Fatalf("invalid streamed zip: %v", err)
	}
}

func TestSizedDownloadUpstreamSizeMismatchFails(t *testing.T) {
	u, files, _ := setupSized(t)
	files[0].data = []byte("hello world, but longer\n") // declared size differs from upstream
	desc := sizedDescriptor(u, files, false)

	zs := httptest.NewServer(zip_streamer.NewServer())
	defer zs.Close()
	resp, err := http.Post(zs.URL+"/download", "application/json", bytes.NewReader(desc))
	if err != nil {
		return // connection error is fine
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Errorf("expected truncated response error")
	}
}

func TestDescriptorCRCParsing(t *testing.T) {
	cases := map[string]struct {
		crc   uint32
		known bool
	}{
		`"8a3f1b2c"`:   {0x8a3f1b2c, true},
		`"0x8A3F1B2C"`: {0x8a3f1b2c, true},
		`2319391532`:   {0x8a3f1b2c, true},
		`"-"`:          {0, false},
		`""`:           {0, false},
		`null`:         {0, false},
	}
	for crcJson, want := range cases {
		desc := []byte(`{"files":[{"url":"https://example.com/a","zipPath":"a","size":5,"crc32":` + crcJson + `}]}`)
		zd, err := zip_streamer.UnmarshalJsonZipDescriptor(desc)
		if err != nil {
			t.Fatalf("%s: %v", crcJson, err)
		}
		crc, known := zd.Files()[0].CRC32()
		if crc != want.crc || known != want.known {
			t.Errorf("%s: got %x/%v", crcJson, crc, known)
		}
	}

	for _, bad := range []string{`"xyz"`, `"123456789"`, `-1`, `4294967296`} {
		desc := []byte(`{"files":[{"url":"https://example.com/a","zipPath":"a","size":5,"crc32":` + bad + `}]}`)
		if _, err := zip_streamer.UnmarshalJsonZipDescriptor(desc); err == nil {
			t.Errorf("%s: expected error", bad)
		}
	}
}

func TestZip64ManyEntries(t *testing.T) {
	// More than 65535 entries requires zip64 end of central directory records.
	// Empty files don't need any upstream fetch.
	var sb strings.Builder
	sb.WriteString(`{"files":[`)
	const n = 70000
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"url":"http://127.0.0.1:1/x","zipPath":"f%d","size":0,"crc32":"00000000"}`, i)
	}
	sb.WriteString(`]}`)

	rr := doRequest(t, "POST", []byte(sb.String()), nil)
	checkHttpOk(rr, t)
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != n || zr.File[n-1].Name != fmt.Sprintf("f%d", n-1) {
		t.Fatalf("got %d files", len(zr.File))
	}
}

// zeroReader is an io.ReadSeeker of a large, all-zero file.
type zeroReader struct{ size, pos int64 }

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.pos >= z.size {
		return 0, io.EOF
	}
	if int64(len(p)) > z.size-z.pos {
		p = p[:z.size-z.pos]
	}
	clear(p)
	z.pos += int64(len(p))
	return len(p), nil
}

func (z *zeroReader) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		z.pos = off
	case io.SeekCurrent:
		z.pos += off
	case io.SeekEnd:
		z.pos = z.size + off
	}
	return z.pos, nil
}

// httpReaderAt reads a remote file using HTTP Range requests.
type httpReaderAt struct {
	t   *testing.T
	url string
}

func (h *httpReaderAt) ReadAt(p []byte, off int64) (int, error) {
	req, _ := http.NewRequest("GET", h.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadFull(resp.Body, p)
}

func TestZip64LargeFilesViaRanges(t *testing.T) {
	// Two files of 5 GiB: needs zip64 sizes and offsets. The archive is only
	// accessed with range requests, so nothing close to 10 GiB is transferred.
	const big = 5 << 30
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size := int64(big)
		if r.URL.Path == "/3" {
			size = 10
		}
		http.ServeContent(w, r, "", time.Time{}, &zeroReader{size: size})
	}))
	defer upstreamSrv.Close()

	desc := []byte(fmt.Sprintf(`{"files":[
		{"url":"%s/1","zipPath":"big1.bin","size":%d,"crc32":"12345678"},
		{"url":"%s/2","zipPath":"big2.bin","size":%d,"crc32":"9abcdef0"},
		{"url":"%s/3","zipPath":"small.bin","size":10,"crc32":"e38a6876"}]}`,
		upstreamSrv.URL, big, upstreamSrv.URL, big, upstreamSrv.URL))

	server := zip_streamer.NewServer()
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest("POST", "/create_download_link", bytes.NewReader(desc)))
	var resp struct {
		LinkId string `json:"link_id"`
		Size   int64  `json:"size"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Size <= 2*big {
		t.Fatalf("unexpected size %d", resp.Size)
	}

	zs := httptest.NewServer(server)
	defer zs.Close()
	zr, err := zip.NewReader(&httpReaderAt{t: t, url: zs.URL + "/download_link/" + resp.LinkId}, resp.Size)
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 3 {
		t.Fatalf("got %d files", len(zr.File))
	}
	wantCRC := []uint32{0x12345678, 0x9abcdef0, 0xe38a6876}
	wantSize := []uint64{big, big, 10}
	for i, f := range zr.File {
		if f.UncompressedSize64 != wantSize[i] || f.CompressedSize64 != wantSize[i] || f.CRC32 != wantCRC[i] {
			t.Errorf("file %d: size %d/%d crc %x", i, f.UncompressedSize64, f.CompressedSize64, f.CRC32)
		}
		off, err := f.DataOffset() // reads the local header via range request
		if err != nil {
			t.Fatalf("file %d: %v", i, err)
		}
		if i == 2 && off <= 2*big {
			t.Errorf("small file offset %d", off)
		}
	}

	// The small file after the zip64 offsets is fully readable (incl. CRC check)
	rc, err := zr.File[2].Open()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rc)
	if err != nil || !bytes.Equal(data, make([]byte, 10)) {
		t.Errorf("small file: %v", err)
	}
}
