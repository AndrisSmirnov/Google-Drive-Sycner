package drive

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	mimeFolder   = "application/vnd.google-apps.folder"
	mimeShortcut = "application/vnd.google-apps.shortcut"
	mimeGoogle   = "application/vnd.google-apps."
)

// File is what Drive says about one item: a file, a folder, a Google
// document or a shortcut.
type File struct {
	ID       string
	Name     string
	MimeType string
	// Size is the content size of a file. For a Google document it is what
	// the document takes on Drive, not the size of its export.
	Size     int64
	Created  time.Time
	Modified time.Time
	// MD5 is the checksum of the content; Google documents have none.
	MD5 string
	// Parent is the folder the item is in. It may name a folder the account
	// cannot see: then the item is one shared with the account directly.
	Parent string
	// CanDownload is false when the owner has forbidden downloading.
	CanDownload bool
}

// IsFolder tells a folder from everything else.
func (f File) IsFolder() bool { return f.MimeType == mimeFolder }

// IsShortcut tells a pointer to a file that lives elsewhere.
func (f File) IsShortcut() bool { return f.MimeType == mimeShortcut }

// GoogleType is "document", "spreadsheet", "presentation", "form" and so on
// for an item made in a Google editor, and empty for an ordinary file.
func (f File) GoogleType() string {
	if f.IsFolder() || f.IsShortcut() || !strings.HasPrefix(f.MimeType, mimeGoogle) {
		return ""
	}
	return strings.TrimPrefix(f.MimeType, mimeGoogle)
}

const listFields = "nextPageToken,files(id,name,mimeType,size,createdTime,modifiedTime,md5Checksum,parents,capabilities(canDownload))"

// List returns everything the account can see, in one flat list: the shared
// folders themselves and all that is inside them. Only descriptions are
// fetched, never content. One request brings up to a thousand items, which
// is several times faster than walking folder by folder.
func (c *Client) List(ctx context.Context) ([]File, error) {
	var all []File
	page := ""
	for {
		query := url.Values{
			"q":                         {"trashed = false"},
			"pageSize":                  {"1000"},
			"fields":                    {listFields},
			"supportsAllDrives":         {"true"},
			"includeItemsFromAllDrives": {"true"},
		}
		if page != "" {
			query.Set("pageToken", page)
		}
		var out struct {
			Next  string `json:"nextPageToken"`
			Files []struct {
				ID           string   `json:"id"`
				Name         string   `json:"name"`
				MimeType     string   `json:"mimeType"`
				Size         string   `json:"size"`
				Created      string   `json:"createdTime"`
				Modified     string   `json:"modifiedTime"`
				MD5          string   `json:"md5Checksum"`
				Parents      []string `json:"parents"`
				Capabilities struct {
					CanDownload *bool `json:"canDownload"`
				} `json:"capabilities"`
			} `json:"files"`
		}
		if err := c.getJSON(ctx, "/files", query, &out); err != nil {
			return nil, err
		}
		for _, raw := range out.Files {
			f := File{ID: raw.ID, Name: raw.Name, MimeType: raw.MimeType, MD5: raw.MD5, CanDownload: true}
			f.Size, _ = strconv.ParseInt(raw.Size, 10, 64)
			f.Created, _ = time.Parse(time.RFC3339, raw.Created)
			f.Modified, _ = time.Parse(time.RFC3339, raw.Modified)
			if len(raw.Parents) > 0 {
				f.Parent = raw.Parents[0]
			}
			if allowed := raw.Capabilities.CanDownload; allowed != nil {
				f.CanDownload = *allowed
			}
			all = append(all, f)
		}
		if page = out.Next; page == "" {
			return all, nil
		}
	}
}
