package zip_streamer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Server struct {
	router             *mux.Router
	linkCache          LinkCache
	Compression        bool
	ListfileUrlPrefix  string
	MaxUpstreamRetries int
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
	r.HandleFunc("/download", server.HandleGetDownload).Methods("GET")
	r.HandleFunc("/create_download_link", server.HandleCreateLink).Methods("POST")
	r.HandleFunc("/download_link/{link_id}", server.HandleDownloadLink).Methods("GET")

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

	linkId := uuid.New().String()
	s.linkCache.Set(linkId, fileEntries)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","link_id":"` + linkId + `"}`))
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

	s.streamEntries(zipDescriptor, w)
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

	s.streamEntries(zipDescriptor, w)
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

	s.streamEntries(zipDescriptor, w)
}

func (s *Server) streamEntries(zipDescriptor *ZipDescriptor, w http.ResponseWriter) {
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
	w.WriteHeader(http.StatusOK)
	err = zipStreamer.StreamAllFiles(s.MaxUpstreamRetries)

	if err != nil {
		// Close the connection so the client gets an error instead of 200 but invalid file
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
