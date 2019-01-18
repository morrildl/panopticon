package panopticon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"playground/log"
)

// RepositoryConfig contains photos. Essentially it owns and oversees the directory where photos are stored.
type RepositoryConfig struct {
	BaseDirectory string
}

// Ready prepares the RepositoryConfig for use.
func (repo *RepositoryConfig) Ready() {
	if repo.BaseDirectory == "" {
		panic("empty BaseDirectory")
	}
	var err error
	repo.BaseDirectory, err = filepath.Abs(repo.BaseDirectory)
	if err != nil {
		panic(err)
	}
}

// Store updates the latest image for the given source (camera.) The provided image data will be
// stored to disk, and will replace the previous in-RAM copy as latest from that source. The
// `handle` returned can be used to refer to the image later, e.g. for lookups.
func (repo *RepositoryConfig) Store(source string, kind MediaKind, ext string, data []byte) *Image {
	return CreateImage(source, kind, ext, data)
}

// Latest retrieves the most recent image data received from the indicated source. If no images have
// been received from that source, `ok` will be false.
func (repo *RepositoryConfig) Latest(source string) *Image {
	var latest string
	var latestTime time.Time
	var latestFI os.FileInfo

	for _, pin := range []MediaKind{MediaCollected, MediaMotion} {
		dir := repo.dirFor(source, pin)
		f, err := os.Open(dir)
		if err != nil {
			panic(err)
		}
		defer f.Close()
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
				latestFI = entry
				continue
			}
			if entry.ModTime().After(latestTime) {
				latestTime = entry.ModTime()
				latest = filepath.Join(dir, entry.Name())
				latestFI = entry
			}
		}
	}

	if latest == "" {
		return nil
	}

	latest = repo.canonFile(latest)
	chunks := strings.Split(latest, string(os.PathSeparator))
	file := chunks[len(chunks)-1]
	pin := chunks[len(chunks)-2]
	camID := chunks[len(chunks)-3]
	if camID != source {
		panic(fmt.Errorf("camera ID from '%s' does not match source ('%s' vs. '%s')", latest, camID, source))
	}

	return &Image{
		Handle:    strings.Split(file, ".")[0],
		Source:    source,
		IsPinned:  repo.segmentToMediaKind(pin) == MediaPinned,
		Timestamp: latestTime,
		Kind:      repo.segmentToMediaKind(pin),
		diskPath:  latest,
		stat:      latestFI,
	}
}

// Locate retrieves the bytes for the indicated image.
func (repo *RepositoryConfig) Locate(handle string) *Image {
	for _, kind := range AllKinds {
		for _, camera := range System.Cameras() {
			dir := repo.dirFor(camera.ID, kind)
			f, err := os.Open(dir)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			entries, err := f.Readdir(0)
			if err != nil {
				panic(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), handle) {
					return &Image{
						Handle:    handle,
						Source:    camera.ID,
						Kind:      kind,
						IsPinned:  kind == MediaPinned,
						Timestamp: entry.ModTime(),

						stat:     entry,
						diskPath: filepath.Join(dir, entry.Name()),
					}
				}
			}
		}
	}

	return nil
}

// List returns the handles of all images associated with the indicated source.
func (repo *RepositoryConfig) List(source string) []*Image {
	if System.GetCamera(source) == nil {
		panic(fmt.Errorf("attempt to list unknown camera '%s'", source))
	}

	images := []*Image{}
	for _, kind := range AllKinds {
		dir := repo.dirFor(source, kind)
		f, err := os.Open(dir)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		entries, err := f.Readdir(0)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			images = append(images, &Image{
				Handle:    strings.Split(entry.Name(), ".")[0],
				Source:    source,
				Kind:      kind,
				IsPinned:  kind == MediaPinned,
				Timestamp: entry.ModTime(),
				stat:      entry,
				diskPath:  repo.canonFile(filepath.Join(dir, entry.Name())),
			})
		}
	}

	return images
}

// PurgeBefore removes all images stored prior to a given time. This is used to enforce a rolling
// window of images.
func (repo *RepositoryConfig) PurgeBefore(kind MediaKind, then time.Time) {
	for _, camera := range System.Cameras() {
		dir := repo.dirFor(camera.ID, kind)
		f, err := os.Open(dir)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		entries, err := f.Readdir(0)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				log.Warn("RepositoryConfig.PurgeBefore", fmt.Sprintf("encountered dir '%s' where it shouldn't be", dir))
				continue
			}
			if entry.ModTime().Before(then) {
				file := repo.canonFile(filepath.Join(dir, entry.Name()))
				if err := os.Remove(file); err != nil {
					panic(err)
				}
			}
		}
	}
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
 * Generated media -- basically timelapses -- are constructed daily per some schedule. Only photos
 * from collected and motion sets are eligible to be used to generate images. Generated images are
 * purged after 14 days.
 *
 * Pinned means a user flagged it for preservation. They are never purged. You can pin any of the
 * other types. A pin is a copy operation, resulting in a new image with new handle and new copy on
 * disk (though timestamps are preserved.)
 */

func (repo *RepositoryConfig) segmentToMediaKind(segment string) MediaKind {
	kind, ok := map[string]MediaKind{
		"collected": MediaCollected,
		"motion":    MediaMotion,
		"pinned":    MediaPinned,
		"generated": MediaGenerated,
	}[segment]
	if !ok {
		return MediaUnknown
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
		if os.IsNotExist(err) { // it's okay if it doesn't exist...
			return abs
		}
		panic(err)
	}
	if !stat.IsDir() { // ...but if it DOES exist, it must be a directory
		panic(fmt.Errorf("'%s' is not a directory", dirPath))
	}
	return abs
}

func (repo *RepositoryConfig) canonFile(file string) string {
	dir, _ := filepath.Split(file)
	absDir := repo.canonDir(dir)

	abs, err := filepath.Abs(file)
	if err != nil {
		panic(err)
	}
	if !strings.HasPrefix(abs, absDir) { // note that absDir is already guaranteed beneath BaseDirectory
		panic(fmt.Errorf("'%s' is not beneath BaseDirectory (%s)", file, repo.BaseDirectory))
	}
	stat, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) { // it's okay if it doesn't exist...
			return abs
		}
		panic(err)
	}
	if stat.IsDir() { // ...but if it DOES exist, it must NOT be a directory
		panic(fmt.Errorf("'%s' is a directory", file))
	}
	return abs
}

func (repo *RepositoryConfig) assertDir(dir string) {
	dir = repo.canonDir(dir)
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0770); err != nil {
				panic(err)
			}
		} else {
			panic(err)
		}
	}
}

func (repo *RepositoryConfig) dirFor(camID string, kind MediaKind) string {
	// verify the camera's base directory exists; create if necessary
	p := filepath.Join(repo.BaseDirectory, camID)
	repo.assertDir(p)

	// verify that the specific kind subdir exists; create if necessary
	p = filepath.Join(p, string(kind))
	repo.assertDir(p)

	return p
}
