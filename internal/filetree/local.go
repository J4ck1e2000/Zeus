package filetree

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

type LocalSource struct{}

func (LocalSource) ReadDir(ctx context.Context, location Location) (Listing, error) {
	local, ok := location.(LocalLocation)
	if !ok {
		return Listing{}, fmt.Errorf("local source cannot read %T", location)
	}
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	entries, err := os.ReadDir(local.Path)
	if err != nil {
		return Listing{}, fmt.Errorf("read local directory %q: %w", local.Path, err)
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		kind := EntryFile
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			kind = EntrySymlink
		case entry.IsDir():
			kind = EntryDirectory
		case !entry.Type().IsRegular():
			kind = EntryOther
		}
		result = append(result, Entry{Name: entry.Name(), Kind: kind})
	}
	sortEntries(result)
	return Listing{Location: local, Entries: result}, nil
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		rank := func(kind EntryKind) int {
			if kind == EntryDirectory {
				return 0
			}
			return 1
		}
		if a, b := rank(entries[i].Kind), rank(entries[j].Kind); a != b {
			return a < b
		}
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Name < entries[j].Name
	})
}
