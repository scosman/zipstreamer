package zip_streamer

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type zipEntry struct {
	Url     string
	ZipPath string
}

type zipPayload struct {
	Entries []zipEntry
}

func UnmarshalPayload(payload []byte) ([]*FileEntry, error) {
	var parsed zipPayload
	err := json.Unmarshal(payload, &parsed)
	if err != nil {
		return nil, err
	}

	results := make([]*FileEntry, 0)
	for _, entry := range parsed.Entries {
		fileEntry, err := NewFileEntry(entry.Url, entry.ZipPath)
		if err == nil {
			results = append(results, fileEntry)
		}
	}

	return results, nil
}

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

func (s *Server) parseZipRequest(w http.ResponseWriter, req *http.Request) ([]*FileEntry, error) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"missing body"}`))
		return nil, err
	}

	fileEntries, err := UnmarshalPayload(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"invalid body"}`))
		return nil, err
	}

	return fileEntries, nil
}

func (s *Server) HandlePostDownload(w http.ResponseWriter, req *http.Request) {
	fileEntries, err := s.parseZipRequest(w, req)
	if err != nil {
		return
	}

	s.streamEntries(fileEntries, w)
}

func (s *Server) HandleGetDownload(w http.ResponseWriter, req *http.Request) {
	params := req.URL.Query()
	listfileUrl := params.Get("furl")
	listFileId := params.Get("fid")
	if listfileUrl == "" && s.ListfileUrlPrefix != "" && listFileId != "" {
		listfileUrl = s.ListfileUrlPrefix + listFileId
	}
	if listfileUrl == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","error":"invalid parameters"}`))
		return
	}

	fileEntries, err := retrieveFileEntriesFromUrl(listfileUrl)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":"error","error":"file not found"}`))
		return
	}

	s.streamEntries(fileEntries, w)
}

func retrieveFileEntriesFromUrl(listfileUrl string) ([]*FileEntry, error) {
	listfileResp, err := http.Get(listfileUrl)
	if err != nil {
		return nil, err
	}
	defer listfileResp.Body.Close()
	if listfileResp.StatusCode != http.StatusOK {
		return nil, errors.New("List File Server Errror")
	}
	body, err := ioutil.ReadAll(listfileResp.Body)
	if err != nil {
		return nil, err
	}

	return UnmarshalPayload(body)
}

func (s *Server) HandleDownloadLink(w http.ResponseWriter, req *http.Request) {
	linkId := mux.Vars(req)["link_id"]
	fileEntries := s.linkCache.Get(linkId)
	if fileEntries == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":"error","error":"link not found"}`))
		return
	}

	s.streamEntries(fileEntries, w)
}

func (s *Server) streamEntries(fileEntries []*FileEntry, w http.ResponseWriter) {
	zipStreamer, err := NewZipStream(fileEntries, w)
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
	w.Header().Set("Content-Disposition", "attachment; filename=\"archive.zip\"")
	w.WriteHeader(http.StatusOK)
	err = zipStreamer.StreamAllFiles()

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
