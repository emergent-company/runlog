package form

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// UploadedFile holds a parsed multipart file upload.
type UploadedFile struct {
	File     multipart.File
	Header   *multipart.FileHeader
	Filename string
	Size     int64
}

// ParseUploadedFile extracts a single file from a multipart form.
// Validates maxSizeMB (0 = unlimited). Closes the file on error — caller must
// close on success.
func ParseUploadedFile(r *http.Request, key string, maxSizeMB int) (*UploadedFile, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, fmt.Errorf("invalid multipart form: %w", err)
	}

	file, header, err := r.FormFile(key)
	if err != nil {
		return nil, fmt.Errorf("file field %q not found: %w", key, err)
	}

	if maxSizeMB > 0 {
		maxBytes := int64(maxSizeMB) << 20
		if header.Size > maxBytes {
			file.Close()
			return nil, fmt.Errorf("file exceeds maximum size of %d MB", maxSizeMB)
		}
	}

	return &UploadedFile{
		File:     file,
		Header:   header,
		Filename: header.Filename,
		Size:     header.Size,
	}, nil
}

// ReadAll reads the uploaded file into memory. Closes the file after reading.
func (u *UploadedFile) ReadAll() ([]byte, error) {
	defer u.File.Close()
	return io.ReadAll(u.File)
}

// Accept checks if the filename matches any of the allowed extensions.
// Extensions should include the dot prefix, e.g. ".pdf", ".png".
func (u *UploadedFile) Accept(allowed ...string) bool {
	for _, ext := range allowed {
		if len(u.Filename) >= len(ext) && u.Filename[len(u.Filename)-len(ext):] == ext {
			return true
		}
	}
	return false
}
