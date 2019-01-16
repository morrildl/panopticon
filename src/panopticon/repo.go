package panopticon

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RepositoryConfig contains photos. Essentially it owns and oversees the directory where photos are stored.
type RepositoryConfig struct {
	BaseDirectory string
}

// Store updates the latest image for the given source (camera.) The provided image data will be
// stored to disk, and will replace the previous in-RAM copy as latest from that source. The
// `handle` returned can be used to refer to the image later, e.g. for lookups.
func (repo *RepositoryConfig) Store(source string, pin PinKind, data []byte) *Image {
	panic(errors.New("unimplemented"))
}

// Latest retrieves the most recent image data received from the indicated source. If no images have
// been received from that source, `ok` will be false.
func (repo *RepositoryConfig) Latest(source string) *Image {
	var latest string
	var latestTime time.Time

	for _, pin := range []PinKind{PinNone, PinMotion} {
		dir := repo.dirFor(source, pin)
		f, err := os.Open(dir)
		if err != nil {
			panic(err)
		}
		entries, err := f.Readdir(0)
		if err != nil {
			panic(err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if latest == "" {
				latest = filepath.Join(dir, entry.Name())
				latestTime = entry.ModTime()
				continue
			}
			if entry.ModTime().After(latestTime) {
				latestTime = entry.ModTime()
				latest = filepath.Join(dir, entry.Name())
			}
		}
	}

	if latest == "" {
		return nil
	}

	latest = repo.canonFile(latest)
	dir, file := filepath.Split(latest)
	dir, pin := filepath.Split(dir)
	dir, camID := filepath.Split(dir)
	if camID != source {
		panic(fmt.Errorf("camera ID does not match source ('%s' vs. '%s')", camID, source))
	}

	return &Image{
		Handle:    strings.Split(file, ".")[0],
		Source:    source,
		IsPinned:  repo.segmentToPinKind(pin),
		Timestamp: latestTime,
	}
}

// Locate retrives the bytes for the indicated image.
func (repo *RepositoryConfig) Locate(handle string) *Image {
	panic(errors.New("unimplemented"))
}

// List returns the handles of all images associated with the indicated source.
func (repo *RepositoryConfig) List(source string) []*Image {
	panic(errors.New("unimplemented"))
}

// PurgeBefore removes all images stored prior to a given time. This is used to enforce a rolling
// window of images.
func (repo *RepositoryConfig) PurgeBefore(then time.Time) {
	panic(errors.New("unimplemented"))
}

/*
 * Directory Structure vs. Classifications
 *
 * Collected, i.e. unpinned periodic uploads: {{.BaseDirectory}}/{{.CameraID}}/collected/{{.SHA}}.jpg
 * Motion-pushed: {{.BaseDirectory}}/{{.CameraID}}/motion/{{.SHA}}.jpg
 * Pinned: {{.BaseDirectory}}/{{.CameraID}}/pinned/{{.SHA}}.{{.Extension}}
 * Generated: {{.BaseDirectory}}/{{.CameraID}}/generated/{{.SHA}}.{{.Extension}}
 *
 * Collected images are purged every 24 hours.
 *
 * Motion images are pushed in response to camera-side motion detection. They are purged every 24 hours.
 *
 * Generated media -- basically timelapses -- are constructed daily from the union of collected
 * and motion images. They are purged after 14 days.
 *
 * Pinned means a user flagged it for preservation. They are never purged. You can pin any of the other types.
 */

func (repo *RepositoryConfig) pinKindToSegment(pin PinKind) string {
	segment, ok := map[PinKind]string{
		PinNone:    "collected",
		PinUnknown: "collected",
		PinMotion:  "motion",
		PinFlagged: "pinned",
	}[pin]
	if !ok {
		return "collected"
	}
	return segment
}

func (repo *RepositoryConfig) segmentToPinKind(segment string) PinKind {
	kind, ok := map[string]PinKind{
		"collected": PinNone,
		"motion":    PinMotion,
		"pinned":    PinFlagged,
	}[segment]
	if !ok {
		return PinUnknown
	}
	return kind
}

func (repo *RepositoryConfig) canonDir(dirPath string) string {
	abs, err := filepath.Abs(dirPath)
	if err != nil {
		panic(err)
	}
	if !strings.HasPrefix(abs, repo.BaseDirectory) {
		panic(fmt.Errorf("'%s' is not beneath BaseDirectory (%s)", dirPath, repo.BaseDirectory))
	}
	stat, err := os.Stat(abs)
	if err != nil {
		panic(err)
	}
	if !stat.IsDir() {
		panic(fmt.Errorf("'%s' is not a directory", dirPath))
	}
	return abs
}

func (repo *RepositoryConfig) canonFile(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		panic(err)
	}
	if !strings.HasPrefix(abs, repo.BaseDirectory) {
		panic(fmt.Errorf("'%s' is not beneath BaseDirectory (%s)", file, repo.BaseDirectory))
	}
	stat, err := os.Stat(abs)
	if err != nil {
		panic(err)
	}
	if stat.IsDir() {
		panic(fmt.Errorf("'%s' is a directory", file))
	}
	return abs
}

func (repo *RepositoryConfig) dirFor(camID string, pin PinKind) string {
	fullPath := filepath.Join(repo.BaseDirectory, camID, repo.pinKindToSegment(pin))
	return repo.canonDir(fullPath)
}

// Image represents an image file stored on disk.
type Image struct {
	Handle    string
	Source    string
	Timestamp time.Time
	IsPinned  PinKind
}

// PinKind enumerates the reasons for which an image might be pinned (from purging) by the system.
type PinKind string

// Constants indicating the reason an image was pinned
const (
	PinNone    PinKind = ""
	PinMotion          = "motion"
	PinFlagged         = "flagged"
	PinUnknown         = "unknown"
)

// Retrieve fetches the bytes for this image and stores them in the provided buffer.
func (img *Image) Retrieve(buf *bytes.Buffer) {
	panic(errors.New("unimplemented"))
}

// Erase removes the indicated image from disk.
func (img *Image) Erase() error {
	panic(errors.New("unimplemented"))
}

// Pin sets the Image to be pinned. `reason` must be one of the `Pin*` enum constants.
func (img *Image) Pin(reason PinKind) {
	panic(errors.New("unimplemented"))
}

// PrettyTime returns a cute human-readable version of `img.Timestamp`.
func (img *Image) PrettyTime() string {
	return ""
}
