package drive

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ErrNoResume means Drive sent the file from its start although the rest of
// it was asked for. Whatever was saved before has to be dropped.
var ErrNoResume = errors.New("Google Drive sent the whole file instead of the rest of it")

// Reasons Drive gives for refusing content, as found in APIError.Reason.
const (
	ReasonNotDownloadable = "fileNotDownloadable"     // a Google document: it can only be exported
	ReasonExportTooLarge  = "exportSizeLimitExceeded" // the export is over Drive's limit
	ReasonCannotDownload  = "cannotDownloadFile"      // the owner has forbidden downloading
)

// Download copies the content of a file to w, starting at byte offset, and
// returns how many bytes it wrote. To continue a download that broke off,
// pass the size of the part already saved. If the connection breaks again,
// the bytes written so far are returned together with the error.
func (c *Client) Download(ctx context.Context, id string, offset int64, w io.Writer) (int64, error) {
	header := http.Header{}
	if offset > 0 {
		header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	resp, err := c.open(ctx, "/files/"+url.PathEscape(id), url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}}, header)
	if err != nil {
		// Asking for the rest of a file that is already complete.
		var refused *APIError
		if offset > 0 && errors.As(err, &refused) && refused.Status == http.StatusRequestedRangeNotSatisfiable {
			return 0, nil
		}
		return 0, err
	}
	defer resp.Body.Close()
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		return 0, ErrNoResume
	}
	return io.Copy(w, resp.Body)
}

// exports maps a Google document type to the Office format it is saved in.
var exports = map[string]struct{ ext, mime string }{
	"document":     {".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"spreadsheet":  {".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"presentation": {".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
}

// ExportExt is the extension a Google document gets on the drive: ".docx"
// for a document, ".xlsx" for a spreadsheet, ".pptx" for a presentation.
// Other Google types (forms, sites, drawings) cannot be exported.
func ExportExt(googleType string) (ext string, ok bool) {
	format, ok := exports[googleType]
	return format.ext, ok
}

// Export converts a Google document to its Office format and copies the
// result to w. An export cannot be continued: it always starts over.
func (c *Client) Export(ctx context.Context, id, googleType string, w io.Writer) (int64, error) {
	format, ok := exports[googleType]
	if !ok {
		return 0, errors.New("Google " + googleType + " cannot be exported")
	}
	resp, err := c.open(ctx, "/files/"+url.PathEscape(id)+"/export", url.Values{"mimeType": {format.mime}}, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return io.Copy(w, resp.Body)
}

// Refused tells whether an error is Drive refusing for the given reason.
func Refused(err error, reason string) bool {
	var refused *APIError
	return errors.As(err, &refused) && refused.Reason == reason
}
