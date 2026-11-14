package engine

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/names"
	"syncer-gdrive/internal/state"
)

// Snapshot is Drive as the latest Refresh saw it, shaped for the copy:
// every item has its place on the drive and every file its selection.
type Snapshot struct {
	Items    map[string]state.Item // by identifier; only what can be copied, and folders
	Top      []string              // the items shared with the account directly
	Children map[string][]string   // folder → what is in it, folders first, then by name
	Checked  map[string]bool       // file → selected
	TakeNew  map[string]bool       // folder → takes new items while selected in part; "" is the top level

	// Wanted is where an item belongs on the drive. For a copied file that
	// was renamed or moved on Drive it differs from Items[id].Path, which
	// is where the copy still lies.
	Wanted map[string]string
	// Clash lists selected files that share a name with another selected
	// file of their folder, with the name each has on the drive.
	Clash map[string]string
	// NotCopied counts what Drive holds but the copy never will, by reason.
	NotCopied map[string]Tally
	// NewUnselected counts the files that appeared on Drive since the last
	// look and are not selected.
	NewUnselected Tally
	At            time.Time

	gone []state.Item // known before, missing from Drive now
}

// Selection states of a folder, worked out from the files under it.
const (
	None = iota
	Partial
	All
)

// State tells whether all, some or none of the files under an item are
// selected. ok is false for a folder with no files under it: it has no
// state of its own.
func (s *Snapshot) State(id string) (state int, ok bool) {
	all, none, any := true, true, false
	var walk func(id string)
	walk = func(id string) {
		if !s.Items[id].Folder {
			any = true
			if s.Checked[id] {
				none = false
			} else {
				all = false
			}
			return
		}
		for _, child := range s.Children[id] {
			walk(child)
		}
	}
	walk(id)
	switch {
	case !any:
		return None, false
	case all:
		return All, true
	case none:
		return None, true
	}
	return Partial, true
}

// topState is the state of everything shared with the account.
func (s *Snapshot) topState() (state int, ok bool) {
	all, none, any := true, true, false
	for _, id := range s.Top {
		st, has := s.State(id)
		if !has {
			continue
		}
		any = true
		all = all && st == All
		none = none && st == None
	}
	switch {
	case !any:
		return None, false
	case all:
		return All, true
	case none:
		return None, true
	}
	return Partial, true
}

// takesNew decides whether a file that has just appeared in a folder is
// synced. A folder selected as a whole takes it; one selected in part asks
// its switch; one not selected does not. A folder with no files yet has no
// say, and the question goes to the folder above it. With nothing selected
// or known anywhere — a first run — everything is taken.
func (s *Snapshot) takesNew(folder string) bool {
	for {
		st, ok := s.topState()
		if folder != "" {
			st, ok = s.State(folder)
		}
		if ok {
			switch st {
			case All:
				return true
			case None:
				return false
			}
			on, set := s.TakeNew[folder]
			return !set || on
		}
		if folder == "" {
			return true
		}
		folder = s.Items[folder].Parent
	}
}

// Refresh looks at Drive and brings the state up to date: what is there,
// where each item belongs on the drive, and whether files that are new are
// selected. It copies nothing.
func (e *Engine) Refresh(ctx context.Context) (*Snapshot, error) {
	files, err := e.remote.List(ctx)
	if err != nil {
		return nil, err
	}
	known, err := e.store.Items()
	if err != nil {
		return nil, err
	}
	old := make(map[string]state.Item, len(known))
	for _, it := range known {
		old[it.ID] = it
	}
	checked, err := e.store.Checked()
	if err != nil {
		return nil, err
	}
	takeNew, err := e.store.TakeNew()
	if err != nil {
		return nil, err
	}

	s := &Snapshot{
		Items: map[string]state.Item{}, Children: map[string][]string{}, Checked: checked, TakeNew: takeNew,
		Wanted: map[string]string{}, Clash: map[string]string{}, NotCopied: map[string]Tally{}, At: e.now(),
	}

	// What can be copied at all.
	local := map[string]string{} // id → the name the item asks for on the drive
	for _, f := range files {
		reason := ""
		name := f.Name
		switch google := f.GoogleType(); {
		case f.IsFolder():
		case f.IsShortcut():
			reason = NotShortcut
		case names.IsService(f.Name):
			reason = NotService
		case google != "":
			ext, ok := drive.ExportExt(google)
			if !ok {
				reason = NotUnsupported
			}
			name += ext
		case !f.CanDownload:
			reason = NotForbidden
		}
		if reason != "" {
			t := s.NotCopied[reason]
			t.add(f.Size)
			s.NotCopied[reason] = t
			continue
		}
		s.Items[f.ID] = state.Item{
			ID: f.ID, Parent: f.Parent, Name: f.Name, Folder: f.IsFolder(), Google: f.GoogleType(),
			Size: f.Size, MD5: f.MD5, Created: f.Created, Modified: f.Modified, Seen: true,
		}
		local[f.ID] = name
	}

	// The tree. An item whose parent the account cannot see is one shared
	// with it directly: it stands at the top.
	for id, it := range s.Items {
		if parent, ok := s.Items[it.Parent]; !ok || !parent.Folder {
			it.Parent = ""
			s.Items[id] = it
			s.Top = append(s.Top, id)
		} else {
			s.Children[it.Parent] = append(s.Children[it.Parent], id)
		}
	}

	// Names. An item that is already known under the same Drive name in
	// the same folder keeps the name it has on the drive; the others are
	// named around those, the oldest first.
	assign := func(parent string, ids []string) map[string]string {
		out := map[string]string{}
		taken := map[string]bool{}
		var fresh []names.Item
		for _, id := range ids {
			it := s.Items[id]
			if was, ok := old[id]; ok && was.Name == it.Name && was.Parent == parent && was.Path != "" {
				name := path.Base(was.Path)
				out[id] = name
				taken[names.Key(name)] = true
				continue
			}
			fresh = append(fresh, names.Item{ID: id, Name: local[id], Created: it.Created})
		}
		sort.SliceStable(fresh, func(a, b int) bool {
			if !fresh[a].Created.Equal(fresh[b].Created) {
				return fresh[a].Created.Before(fresh[b].Created)
			}
			return fresh[a].ID < fresh[b].ID
		})
		for _, it := range fresh {
			clean := names.Clean(it.Name)
			name := clean
			for n := 2; taken[names.Key(name)]; n++ {
				name = names.Numbered(clean, n)
			}
			taken[names.Key(name)] = true
			out[it.ID] = name
		}
		return out
	}
	var place func(parent, parentPath string, ids []string)
	place = func(parent, parentPath string, ids []string) {
		for id, name := range assign(parent, ids) {
			s.Wanted[id] = path.Join(parentPath, name)
		}
		for _, id := range ids {
			if s.Items[id].Folder {
				place(id, s.Wanted[id], s.Children[id])
			}
		}
	}
	place("", "", s.Top)

	// Order for showing: folders first, then by the name on the drive.
	order := func(ids []string) {
		sort.Slice(ids, func(a, b int) bool {
			x, y := s.Items[ids[a]], s.Items[ids[b]]
			if x.Folder != y.Folder {
				return x.Folder
			}
			return names.Key(path.Base(s.Wanted[x.ID])) < names.Key(path.Base(s.Wanted[y.ID]))
		})
	}
	order(s.Top)
	for id := range s.Children {
		order(s.Children[id])
	}

	// Selection of files that are new. The decision is made against the
	// selection as it was: a tree of the folders and of the files that
	// already had an answer, without any of the new files.
	before := &Snapshot{Items: s.Items, Children: map[string][]string{}, Checked: checked, TakeNew: takeNew}
	had := func(id string) bool {
		_, decided := checked[id]
		return decided || s.Items[id].Folder
	}
	for _, id := range s.Top {
		if had(id) {
			before.Top = append(before.Top, id)
		}
	}
	for parent, ids := range s.Children {
		for _, id := range ids {
			if had(id) {
				before.Children[parent] = append(before.Children[parent], id)
			}
		}
	}
	decided := map[string]bool{}
	for id, it := range s.Items {
		if _, ok := checked[id]; !ok && !it.Folder {
			decided[id] = before.takesNew(it.Parent)
			if !decided[id] {
				s.NewUnselected.add(it.Size)
			}
		}
	}
	for id, yes := range decided {
		s.Checked[id] = yes
	}

	// Where each item is recorded. A copied file stays recorded where its
	// copy lies until Run has moved it.
	items := make([]state.Item, 0, len(s.Items))
	for id, it := range s.Items {
		it.Path = s.Wanted[id]
		if was, ok := old[id]; ok && was.Copy != nil {
			it.Path = was.Path
			it.Copy = was.Copy
		}
		s.Items[id] = it
		items = append(items, it)
	}
	if err := e.store.SaveListing(items); err != nil {
		return nil, err
	}
	if len(decided) > 0 {
		if err := e.store.SetChecked(decided); err != nil {
			return nil, err
		}
	}
	// Items gone from Drive stay in the state until Run deals with them.
	for id, was := range old {
		if _, still := s.Items[id]; !still {
			was.Seen = false
			s.gone = append(s.gone, was)
		}
	}

	s.findClashes()
	e.last = s
	return s, nil
}

// Known returns Drive as the state remembers it from the last look, without
// asking Drive. It is what can be shown before the first Refresh and when
// there is no connection.
func (e *Engine) Known() (*Snapshot, error) {
	known, err := e.store.Items()
	if err != nil {
		return nil, err
	}
	checked, err := e.store.Checked()
	if err != nil {
		return nil, err
	}
	takeNew, err := e.store.TakeNew()
	if err != nil {
		return nil, err
	}
	s := &Snapshot{
		Items: map[string]state.Item{}, Children: map[string][]string{}, Checked: checked, TakeNew: takeNew,
		Wanted: map[string]string{}, Clash: map[string]string{}, NotCopied: map[string]Tally{},
	}
	for _, it := range known {
		if it.Seen {
			s.Items[it.ID] = it
			s.Wanted[it.ID] = it.Path
		}
	}
	for id, it := range s.Items {
		if parent, ok := s.Items[it.Parent]; ok && parent.Folder {
			s.Children[it.Parent] = append(s.Children[it.Parent], id)
		} else {
			s.Top = append(s.Top, id)
		}
	}
	order := func(ids []string) {
		sort.Slice(ids, func(a, b int) bool {
			x, y := s.Items[ids[a]], s.Items[ids[b]]
			if x.Folder != y.Folder {
				return x.Folder
			}
			return names.Key(path.Base(x.Path)) < names.Key(path.Base(y.Path))
		})
	}
	order(s.Top)
	for id := range s.Children {
		order(s.Children[id])
	}
	s.findClashes()
	if e.last == nil {
		e.last = s
	}
	return s, nil
}

// Latest is what the most recent look found, by Refresh, Plan or Known; nil
// before any of them.
func (e *Engine) Latest() *Snapshot { return e.last }

// LastSync tells when a destination was last synced: zero if it never was,
// or if the folder holds no copy. It does not create anything.
func LastSync(destination string) time.Time {
	if _, err := os.Stat(filepath.Join(destination, state.Dir, "state.db")); err != nil {
		return time.Time{}
	}
	store, err := state.Open(destination)
	if err != nil {
		return time.Time{}
	}
	defer store.Close()
	value, _ := store.Meta(MetaLastSync)
	at, _ := time.Parse(time.RFC3339, value)
	return at
}

// findClashes lists the selected files of each folder that end up under
// one name apart from the number added to tell them apart.
func (s *Snapshot) findClashes() {
	s.Clash = map[string]string{}
	check := func(ids []string) {
		var selected []names.Item
		for _, id := range ids {
			if it := s.Items[id]; !it.Folder && s.Checked[id] {
				name := it.Name
				if ext, ok := drive.ExportExt(it.Google); ok {
					name += ext
				}
				selected = append(selected, names.Item{ID: id, Name: name, Created: it.Created})
			}
		}
		for id := range names.Clashes(selected) {
			s.Clash[id] = path.Base(s.Wanted[id])
		}
	}
	check(s.Top)
	for _, ids := range s.Children {
		check(ids)
	}
}

// SetSelection records what the user chose: which files are selected and
// the switches "new files and folders here".
func (e *Engine) SetSelection(checked, takeNew map[string]bool) error {
	if err := e.store.SetChecked(checked); err != nil {
		return err
	}
	if err := e.store.SetTakeNew(takeNew); err != nil {
		return err
	}
	if e.last != nil {
		for id, yes := range checked {
			e.last.Checked[id] = yes
		}
		for id, yes := range takeNew {
			e.last.TakeNew[id] = yes
		}
		e.last.findClashes()
	}
	return nil
}
