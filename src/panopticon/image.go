package panopticon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"playground/log"
)

// Image represents an image file stored on disk.
type Image struct {
	Handle    string
	Source    string
	Timestamp time.Time
	Kind      MediaKind
	IsPinned  bool

	diskPath string
	stat     os.FileInfo
}

// CreateImage stores the bytes to the disk according to config & convention, and returns a handle to the
// resulting image.
func CreateImage(source string, kind MediaKind, ext string, b []byte) *Image {
	dir := Repository.dirFor(source, kind)

	// compute a hash of the file based on its contents, disk location, and current time
	timestamp := time.Now()
	tb, err := timestamp.MarshalBinary()
	potato := sha256.New()
	potato.Write(b)
	potato.Write(tb)
	potato.Write([]byte(dir))
	handle := hex.EncodeToString(potato.Sum(nil)[:32])

	// verify that the file doesn't somehow already exist
	diskPath := Repository.canonFile(filepath.Join(dir, fmt.Sprintf("%s.%s", handle, ext)))
	_, err = os.Stat(diskPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error("Image.CreateImage", "file hash collision?! '%s'", handle)
			panic(err)
		}
	}

	// write the actual file contents under its computed name
	if err := ioutil.WriteFile(diskPath, b, 0660); err != nil {
		panic(err)
	}
	stat, err := os.Stat(diskPath)
	if err != nil {
		panic(err)
	}

	return &Image{
		Handle:    handle,
		Source:    source,
		Kind:      kind,
		Timestamp: timestamp,
		IsPinned:  kind == MediaPinned,
		stat:      stat,
		diskPath:  diskPath,
	}
}

// Retrieve fetches the bytes for this image and stores them in the provided buffer.
func (img *Image) Retrieve(buf *bytes.Buffer) {
	if img.diskPath == "" {
		panic(fmt.Errorf("missing diskPath for image '%s'", img.Handle))
	}
	diskPath := Repository.canonFile(img.diskPath)
	if diskPath != img.diskPath {
		panic(fmt.Errorf("image diskPath does not resolve to itself '%s'/'%s'", diskPath, img.diskPath))
	}
	b, err := ioutil.ReadFile(img.diskPath)
	if err != nil {
		panic(err)
	}
	n, err := buf.Write(b)
	if n != len(b) {
		panic(fmt.Errorf("partial write to memory buffer"))
	}
}

// Erase removes the indicated image from disk.
func (img *Image) Erase() {
	if img.diskPath == "" {
		panic(fmt.Errorf("missing diskPath for image '%s'", img.Handle))
	}
	diskPath := Repository.canonFile(img.diskPath)
	if diskPath != img.diskPath {
		panic(fmt.Errorf("image diskPath does not resolve to itself '%s'/'%s'", diskPath, img.diskPath))
	}
	if err := os.Remove(img.diskPath); err != nil {
		if !os.IsNotExist(err) { // ignore if we were just asked to remove a thing which isn't there
			panic(err)
		}
	}
}

// Pin sets the Image to be pinned. `reason` must be one of the `Pin*` enum constants. This is a
// *copy* operation, not a move. As the non-pinned Kinds are subject to periodic purging, it is not
// necessary to delete them. Meanwhile, doing so can result in missing inputs into generated images,
// as the generation behaviors generally do not consider pinned content for inclusion.
//
// The returned `*Image` is for the new pinned copy. Its `Handle` will have changed.
func (img *Image) Pin() *Image {
	// basic sanity checks
	if img.diskPath == "" {
		panic(fmt.Errorf("missing diskPath for image '%s'", img.Handle))
	}
	diskPath := Repository.canonFile(img.diskPath)
	if diskPath != img.diskPath {
		panic(fmt.Errorf("image diskPath does not resolve to itself '%s'/'%s'", diskPath, img.diskPath))
	}

	// extract file name, kind, and camera/source using our convention
	dir, file := filepath.Split(diskPath)
	dir, kind := filepath.Split(dir)
	_, camera := filepath.Split(dir)

	// more sanity checks to make sure decomposition of path didn't go awry
	if Repository.segmentToMediaKind(kind) != img.Kind {
		panic(fmt.Errorf("diskPath does not resolve kind '%s'/'%s'", kind, img.Kind))
	}
	if camera != img.Source { // sanity check
		panic(fmt.Errorf("camera does not resolve '%s'/'%s'", camera, img.Source))
	}

	// read the file contents
	b, err := ioutil.ReadFile(img.diskPath)
	if err != nil {
		panic(err)
	}

	// compute the new disk location; start with directory
	destDir := Repository.dirFor(camera, MediaPinned)

	// compute new handle, which is the SHA256 hash of file contents + current time + parent directory
	tb, _ := time.Now().MarshalBinary()
	potato := sha256.New()
	potato.Write(b)
	potato.Write(tb)
	hash := potato.Sum([]byte(destDir))
	newHandle := hex.EncodeToString(hash)

	// use the same extension (so that a .jpg remains a .jpg etc.)
	chunks := strings.SplitN(file, ".", 2)
	if len(chunks) != 2 {
		panic(fmt.Errorf("unable to decompose filename '%s'", file))
	}

	// now we can construct the fully qualified destination filename
	destFile := Repository.canonFile(filepath.Join(destDir, fmt.Sprintf("%s.%s", newHandle, chunks[1])))

	// write it, and make sure we carry over the timestamp to the new copy
	err = ioutil.WriteFile(destFile, b, 0660)
	if err != nil {
		panic(err)
	}
	if err := os.Chtimes(destFile, img.Timestamp, img.Timestamp); err != nil {
		panic(err)
	}

	// sanity check to make sure it's there
	newStat, err := os.Stat(destFile)
	if err != nil {
		panic(err)
	}

	// return a handle to the new image
	return &Image{
		Handle:    newHandle,
		Source:    camera,
		IsPinned:  true,
		Kind:      MediaPinned,
		Timestamp: img.Timestamp,
		stat:      newStat,
		diskPath:  destFile,
	}
}

// PrettyTime returns a cute human-readable version of the hours and minutes of `img.Timestamp`.
func (img *Image) PrettyTime() string {
	return img.Timestamp.Format("3:04pm")
}

// PrettyDate returns a cute human-readable version of of the full date of `img.Timestamp`.
func (img *Image) PrettyDate() string {
	return img.Timestamp.Format("Monday, 2 January, 2006")
}
