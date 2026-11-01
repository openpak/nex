package api

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by a Store for a missing course, maker or thumbnail.
var ErrNotFound = errors.New("not found")

// Course is a course as the API shows it.
type Course struct {
	DataID    uint64
	Name      string
	OwnerPID  int32
	MakerName string
	Tags      []string
	Created   time.Time
	Updated   time.Time
	Stars     int
	Plays     int
	Clears    int
	Attempts  int
	Failures  int
	// Comments is the game's own count of Miiverse comments (object rating slot 6).
	Comments     int
	HasThumbnail bool
}

// Maker is a maker as the API shows it.
type Maker struct {
	PID     int32
	Name    string
	Courses int
	Stars   int
	Plays   int
	Created time.Time
}

// Sort orders of CourseQuery.
const (
	SortNewest = "newest"
	SortStars  = "stars"
	SortPlays  = "plays"
	SortClears = "clears"
	SortRandom = "random"
)

// CourseQuery selects courses.
type CourseQuery struct {
	Sort         string
	Limit        int
	Offset       int
	UpdatedSince time.Time
	Name         string // contains, ignoring case
	OwnerPID     int32  // 0 for any maker
	FailMin      *int   // failure rate band, inclusive; unattempted courses are left out
	FailMax      *int
}

// Store is the data behind the API.
type Store interface {
	Courses(ctx context.Context, q CourseQuery) ([]Course, int, error)
	Course(ctx context.Context, dataID uint64) (Course, error)
	Thumbnail(ctx context.Context, dataID uint64) ([]byte, error)
	Makers(ctx context.Context, limit, offset int) ([]Maker, int, error)
	// Pickup lists data IDs for the Dothub pickup list: failure rate inside [min, max], most starred first.
	Pickup(ctx context.Context, failMin, failMax, limit int) ([]uint64, error)

	// Bookmarks. A bookmark belongs to a Wii U account (the NEX PID the game logs in with).
	AddBookmark(ctx context.Context, pid int32, dataID uint64) error
	RemoveBookmark(ctx context.Context, pid int32, dataID uint64) error
	Bookmarks(ctx context.Context, pid int32) ([]uint64, error)
	// ResolveNNID finds the PID of a maker by the name the game uploaded as its maker profile.
	ResolveNNID(ctx context.Context, name string) (int32, error)
}
