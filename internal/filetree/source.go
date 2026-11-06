package filetree

import (
	"context"
	"fmt"
)

type EntryKind uint8

const (
	EntryDirectory EntryKind = iota
	EntryFile
	EntrySymlink
	EntryOther
)

type Entry struct {
	Name string
	Kind EntryKind
}

type Listing struct {
	Location Location
	Entries  []Entry
}

type Source interface {
	ReadDir(context.Context, Location) (Listing, error)
}

// Sources dispatches typed locations to the matching local or SSH reader.
type Sources struct {
	Local LocalSource
	SSH   SFTPSource
}

func NewSource() Source { return Sources{} }

func (s Sources) ReadDir(ctx context.Context, location Location) (Listing, error) {
	switch location.(type) {
	case LocalLocation:
		return s.Local.ReadDir(ctx, location)
	case SSHLocation, SSHLocationUnknown:
		return s.SSH.ReadDir(ctx, location)
	default:
		return Listing{}, fmt.Errorf("unsupported file location %T", location)
	}
}
