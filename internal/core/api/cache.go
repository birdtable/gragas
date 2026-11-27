package coreapi

// What gragas keeps on this machine, listed: each kind, its size, its oldest
// entry and the age at which it is cleared by default. The call reads through
// the Tree port, and reads each journal file's records with the Journal's own
// reader of its bytes, and writes nothing (cache clear follows with the
// Writer's removal). Nothing it reads carries a project's name: the files are
// named by digests.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// CacheSchema is the schema number of CacheListing.
const CacheSchema = 1

// CacheKind is one kind to list, as the registry in the settings package
// declares it; the call is handed it, so it imports no settings.
type CacheKind struct {
	Name, Holds string
	// Dir is the folder the kind lives in, "" when the machine has none.
	Dir string
	// Names are the files of the kind, as path.Match patterns relative to Dir.
	Names []string
	// Journal is whether the files are journal files, which say how old an
	// entry is.
	Journal bool
	// Built is whether this build fills the kind.
	Built bool
	// Nested is whether a name matches the leading folders of a path, so a
	// file at any depth below a folder a name matches belongs to the kind:
	// the scratch kind names a folder a run made, and its files lie below it.
	Nested bool
}

// CacheAsk is what a listing is asked for: the kinds, and how long they are
// kept, as the keep setting says it and in seconds.
type CacheAsk struct {
	Kinds       []CacheKind
	Keep        string
	KeepSeconds int64
}

// The types below live in the model, so the kit can name them.
type (
	// CacheListing is the answer of a cache listing, as the model holds it.
	CacheListing = model.CacheListing
	// CacheKindRow is one kind of the cache, as the model holds it.
	CacheKindRow = model.CacheKindRow
)

// CacheList lists what the kinds hold. It fails only on a read that went
// wrong; a folder that is not there holds nothing.
func CacheList(ctx context.Context, ports port.Ports, ask CacheAsk) (CacheListing, error) {
	out := CacheListing{Schema: CacheSchema, Keep: ask.Keep, Kinds: make([]CacheKindRow, 0, len(ask.Kinds))}
	now := ports.Clock.Now().Unix()
	for _, k := range ask.Kinds {
		if err := ctx.Err(); err != nil {
			return CacheListing{}, err
		}
		row := CacheKindRow{Kind: k.Name, Holds: k.Holds, Built: k.Built, ClearedAfter: ask.Keep}
		if err := fill(ctx, ports, k, &row, now); err != nil {
			return CacheListing{}, fmt.Errorf("%s: %w", k.Name, err)
		}
		out.Kinds = append(out.Kinds, row)
	}
	return out, nil
}

// fill reads one kind into row.
func fill(ctx context.Context, ports port.Ports, k CacheKind, row *CacheKindRow, now int64) error {
	tree := ports.Tree
	nothing := func(why string) {
		row.Oldest, row.Age, row.OldestNote = nil, nil, why
	}
	if k.Dir == "" {
		row.SizeNote = "gragas has no folder for this: neither its XDG variable nor HOME is set"
		nothing(row.SizeNote)
		return nil
	}
	root, err := tree.Open(k.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		zero := int64(0)
		row.Size = &zero
		if k.Built {
			nothing("nothing is kept")
		} else {
			nothing("this build keeps none: it is filled by a later release")
		}
		return nil
	}
	if err != nil {
		return err
	}
	// The journal's reader judges the state folder as every call of the
	// Journal does, so it is asked first, with no bytes, before a file there
	// is listed or read: a state folder a clone carries ends the listing,
	// matching port.ErrCarried, and no planted journal is counted.
	if k.Journal {
		if _, _, err := ports.Journal.RecordsOf(ctx, nil); err != nil {
			return err
		}
	}
	entries, err := tree.List(root, "", model.Every)
	if err != nil {
		return err
	}
	var size int64
	var journals []string
	for _, e := range entries {
		if e.Dir || !named(k, e.Path) {
			continue
		}
		row.Files++
		size += e.Size
		if k.Journal && path.Ext(e.Path) == ".journal" {
			journals = append(journals, e.Path)
		}
	}
	row.Size = &size
	switch {
	case !k.Journal && row.Files == 0 && !k.Built:
		nothing("this build keeps none: it is filled by a later release")
	case !k.Journal && row.Files == 0:
		nothing("nothing is kept")
	case !k.Journal:
		nothing("this kind carries no record of when an entry was made")
	default:
		return fillRuns(ctx, ports, root, journals, row, now)
	}
	return nil
}

// named is whether name is a file of the kind: one of its patterns, or, for a
// nested kind, a path below a folder one matches. path.Match stops at a slash,
// so a nested name is matched on as many leading folders as the pattern has
// parts, and the files below the last of them come with it.
func named(k CacheKind, name string) bool {
	return slices.ContainsFunc(k.Names, func(p string) bool {
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
		if !k.Nested {
			return false
		}
		parts := strings.SplitN(name, "/", strings.Count(p, "/")+2)
		if len(parts) <= strings.Count(p, "/")+1 {
			return false // no file below a folder the name could match
		}
		ok, err := path.Match(p, strings.Join(parts[:len(parts)-1], "/"))
		return err == nil && ok
	})
}

// tally is what the journals add up to: the runs without a close, the
// oldest close, and why a journal was not read, by the journal's own reader.
type tally struct {
	open   map[string]bool
	oldest int64
	closed int
	newer  int
	older  []string
}

// add takes the records of one journal, in the order its reader answers
// them: a run's intent opens it and its close closes it.
func (t *tally) add(records []model.Record) {
	for _, r := range records {
		switch r.Kind {
		case model.KindIntent:
			t.open[r.Run] = true
		case model.KindClose:
			delete(t.open, r.Run)
			if t.closed == 0 || r.At < t.oldest {
				t.oldest = r.At
			}
			t.closed++
		}
	}
}

// fillRuns reads the journal files for how old the oldest closed run is and
// how many runs are open. Each file is read by the journal's own reader
// so the listing reads what Records reads. A journal it
// refuses, of a newer version or of an older release's form, is said so in
// the row's note, the age unknown rather than wrong, and the open runs are
// those of the journals read; the listing goes on. A cancel ends it.
func fillRuns(ctx context.Context, ports port.Ports, root model.Root, files []string, row *CacheKindRow, now int64) error {
	t := tally{open: map[string]bool{}}
	for _, f := range files {
		data, err := ports.Tree.Read(root, f)
		if err != nil {
			return err
		}
		records, _, err := ports.Journal.RecordsOf(ctx, data)
		newer, isNewer := errors.AsType[*model.NewerRecordError](err)
		_, isOlder := errors.AsType[*model.OlderJournalError](err)
		switch {
		case isNewer:
			t.newer = max(t.newer, newer.Found)
		case isOlder:
			t.older = append(t.older, f)
		case err != nil:
			return err
		default:
			t.add(records)
		}
	}
	n := len(t.open)
	row.Open = &n
	var notes []string
	if t.newer > 0 {
		notes = append(notes, fmt.Sprintf("a record is version %d and this engine reads up to version %d", t.newer, model.RecordVersion))
	}
	if len(t.older) > 0 {
		notes = append(notes, fmt.Sprintf("%s is of an older release's form, which this engine does not read: recover its runs with that release (gragas journal recover), then remove it", strings.Join(t.older, ", ")))
	}
	switch {
	case len(notes) > 0:
		row.OldestNote = strings.Join(notes, "; ")
	case t.closed == 0:
		row.OldestNote = "no run has closed"
	default:
		age := max(now-t.oldest, 0)
		row.Oldest, row.Age = &t.oldest, &age
	}
	return nil
}
