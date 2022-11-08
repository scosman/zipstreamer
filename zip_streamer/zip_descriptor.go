package zip_streamer

import (
	"encoding/json"
	"strings"
)

type ZipDescriptor struct {
	suggestedFilenameRaw string
	files                []*FileEntry
}

func NewZipDescriptor() *ZipDescriptor {
	zd := ZipDescriptor{
		suggestedFilenameRaw: "",
		files:                make([]*FileEntry, 0),
	}

	return &zd
}

// Filename limited to US-ASCII per https://www.rfc-editor.org/rfc/rfc2183#section-2.3
// Filter " as it's used to quote this filename
func (zd ZipDescriptor) EscapedSuggestedFilename() string {
	rawFilename := zd.suggestedFilenameRaw
	escapedFilenameBuilder := make([]rune, 0, len(rawFilename))
	for _, r := range rawFilename {
		// Printable ASCII chars, no double quote
		if r > 31 && r < 127 && r != '"' {
			escapedFilenameBuilder = append(escapedFilenameBuilder, r)
		}
	}
	escapedFilename := string(escapedFilenameBuilder)

	if escapedFilename != "" && escapedFilename != ".zip" {
		if strings.HasSuffix(escapedFilename, ".zip") {
			return escapedFilename
		} else {
			return escapedFilename + ".zip"
		}
	}

	return "archive.zip"
}

func (zd ZipDescriptor) Files() []*FileEntry {
	return zd.files
}

type jsonZipEntry struct {
	Url     string `json:"Url"`
	ZipPath string `json:"ZipPath"`
}

type jsonZipPayload struct {
	Entries           []jsonZipEntry `json:"entries"`
	SuggestedFilename string         `json:"suggestedFilename"`
}

func UnmarshalJsonZipDescriptor(payload []byte) (*ZipDescriptor, error) {
	var parsed jsonZipPayload
	err := json.Unmarshal(payload, &parsed)
	if err != nil {
		return nil, err
	}

	zd := NewZipDescriptor()
	zd.suggestedFilenameRaw = parsed.SuggestedFilename
	for _, entry := range parsed.Entries {
		fileEntry, err := NewFileEntry(entry.Url, entry.ZipPath)
		if err == nil {
			zd.files = append(zd.files, fileEntry)
		}
	}

	return zd, nil
}
