package zip_streamer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Server struct {
	router            *mux.Router
	linkCache         LinkCache
	Compression       bool
	ListfileUrlPrefix string
}

func NewServer() *Server {
	r := mux.NewRouter()

	timeout := time.Second * 60
	server := Server{
		router:      r,
		linkCache:   NewLinkCache(&timeout),
		Compression: false,
	}

	r.HandleFunc("/download", server.HandlePostDownload).Methods("POST")
	r.HandleFunc("/download", server.HandleGetDownload).Methods("GET", "HEAD")
	r.HandleFunc("/create_download_link", server.HandleCreateLink).Methods("POST")
	r.HandleFunc("/download_link/{link_id}", server.HandleDownloadLink).Methods("GET", "HEAD")

	return &server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originsOk := handlers.AllowedOrigins([]string{"*"})
	headersOk := handlers.AllowedHeaders([]string{"Content-Type", "X-Requested-With", "*"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})
	handlers.CORS(originsOk, headersOk, methodsOk)(s.router).ServeHTTP(w, r)
}

func (s *Server) HandleCreateLink(w http.ResponseWriter, req *http.Request) {
	fileEntries, err := s.parseZipRequest(w, req)
	if err != nil {
		return
	}
	if len(fileEntries.files) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"no files to download"}`))
		return
	}

	linkId := uuid.New().String()
	s.linkCache.Set(linkId, fileEntries)
	sizeField := ""
	if layout := s.zipLayout(fileEntries); layout != nil {
		sizeField = `,"size":` + strconv.FormatInt(layout.Size(), 10)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","link_id":"` + linkId + `"` + sizeField + `}`))
}

func (s *Server) parseZipRequest(w http.ResponseWriter, req *http.Request) (*ZipDescriptor, error) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"missing body"}`))
		return nil, err
	}

	ZipDescriptor, err := UnmarshalJsonZipDescriptor(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"invalid body"}`))
		return nil, err
	}

	return ZipDescriptor, nil
}

func (s *Server) HandlePostDownload(w http.ResponseWriter, req *http.Request) {
	zipDescriptor, err := s.parseZipRequest(w, req)
	if err != nil {
		return
	}

	s.streamEntries(zipDescriptor, w, req)
}

func (s *Server) HandleGetDownload(w http.ResponseWriter, req *http.Request) {
	params := req.URL.Query()
	listfileUrl := params.Get("zsurl")
	listFileId := params.Get("zsid")
	if listfileUrl == "" && s.ListfileUrlPrefix != "" && listFileId != "" {
		listfileUrl = s.ListfileUrlPrefix + listFileId
	}
	if listfileUrl == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"invalid parameters"}`))
		return
	}

	zipDescriptor, err := retrieveZipDescriptorFromUrl(listfileUrl)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":"error","error":"file not found"}`))
		return
	}

	s.streamEntries(zipDescriptor, w, req)
}

func retrieveZipDescriptorFromUrl(listfileUrl string) (*ZipDescriptor, error) {
	listfileResp, err := http.Get(listfileUrl)
	if err != nil {
		return nil, err
	}
	defer listfileResp.Body.Close()
	if listfileResp.StatusCode != http.StatusOK {
		return nil, errors.New("List File Server Error")
	}
	body, err := ioutil.ReadAll(listfileResp.Body)
	if err != nil {
		return nil, err
	}

	return UnmarshalJsonZipDescriptor(body)
}

func (s *Server) HandleDownloadLink(w http.ResponseWriter, req *http.Request) {
	linkId := mux.Vars(req)["link_id"]
	zipDescriptor := s.linkCache.Get(linkId)
	if zipDescriptor == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":"error","error":"link not found"}`))
		return
	}

	s.streamEntries(zipDescriptor, w, req)
}

// maxRanges limits the number of ranges in a single Range request. Each
// range may cause an upstream request, so many tiny ranges are expensive.
const maxRanges = 16

// zipLayout returns the precomputed archive layout if the exact zip bytes
// can be known up front (no compression, size and crc32 for every file),
// otherwise nil.
func (s *Server) zipLayout(zipDescriptor *ZipDescriptor) *ZipLayout {
	if s.Compression || !CanBuildZipLayout(zipDescriptor.Files()) {
		return nil
	}
	layout, err := NewZipLayout(zipDescriptor.Files())
	if err != nil {
		return nil
	}
	return layout
}

func (s *Server) streamEntries(zipDescriptor *ZipDescriptor, w http.ResponseWriter, req *http.Request) {
	if layout := s.zipLayout(zipDescriptor); layout != nil {
		s.serveZipLayout(layout, zipDescriptor, w, req)
		return
	}

	zipStreamer, err := NewZipStream(zipDescriptor.Files(), w)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"invalid entries"}`))
		return
	}

	if s.Compression {
		zipStreamer.CompressionMethod = zip.Deflate
	}

	// need to write the header before bytes
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipDescriptor.EscapedSuggestedFilename()))
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)
	if req.Method == http.MethodHead {
		return
	}
	err = zipStreamer.StreamAllFiles()

	if err != nil {
		// Close the connection so the client gets an error instead of 200 but invalid file
		closeForError(w)
	}
}

// serveZipLayout serves an archive with known layout, supporting
// Content-Length, HEAD, Range and If-Range requests.
func (s *Server) serveZipLayout(layout *ZipLayout, zipDescriptor *ZipDescriptor, w http.ResponseWriter, req *http.Request) {
	// Range handling is only defined for GET (and HEAD) per RFC 9110.
	if (req.Method != http.MethodGet && req.Method != http.MethodHead) ||
		strings.Count(req.Header.Get("Range"), ",") >= maxRanges {
		req.Header.Del("Range")
		req.Header.Del("If-Range")
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipDescriptor.EscapedSuggestedFilename()))
	w.Header().Set("ETag", layout.ETag())

	reader := NewZipLayoutReader(req.Context(), layout)
	defer reader.Close()
	http.ServeContent(w, req, "", time.Time{}, reader)

	if reader.Err() != nil {
		// Close the connection so the client gets an error instead of a truncated file
		closeForError(w)
	}
}

func closeForError(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)

	if !ok {
		return
	}

	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}

	conn.Close()
}
