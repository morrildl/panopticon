// Copyright © 2019 Dan Morrill
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package panopticon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io/ioutil"
	"math"
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
	HasVideo  bool

	diskPath string
	stat     os.FileInfo
}

// CreateImage stores the bytes to the disk according to config & convention, and returns a handle to the
// resulting image.
func CreateImage(source string, kind MediaKind, b []byte) *Image {
	dir := Repository.dirFor(source, kind)

	// compute a hash of the file based on its contents, disk location, and current time
	timestamp := time.Now()
	tb, err := timestamp.MarshalBinary()
	potato := sha256.New()
	potato.Write(b)
	potato.Write(tb)
	potato.Write([]byte(dir))
	handle := hex.EncodeToString(potato.Sum(nil)[:32])

	cam := System.GetCamera(source)
	if cam.Dewarp {
		b = dewarpFisheye(b)
	}

	// verify that the file doesn't somehow already exist
	diskPath := Repository.canonFile(filepath.Join(dir, fmt.Sprintf("%s.%s", handle, "jpg")))
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
		HasVideo:  false,
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
	chunks := strings.Split(diskPath, string(os.PathSeparator))
	if len(chunks) < 4 {
		panic(fmt.Errorf("impossible disk path '%s'", diskPath))
	}
	file := chunks[len(chunks)-1]
	kind := chunks[len(chunks)-2]
	camera := chunks[len(chunks)-3]
	if file == "" || kind == "" || camera == "" {
		panic(fmt.Errorf("missing file/kind/camera '%s'/'%s'/'%s'", file, kind, camera))
	}

	// more sanity checks to make sure decomposition of path didn't go awry
	if Repository.segmentToMediaKind(kind) != img.Kind {
		panic(fmt.Errorf("diskPath (%s) does not resolve kind '%s'/'%s'", diskPath, kind, img.Kind))
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
	timestamp := time.Now()
	tb, err := timestamp.MarshalBinary()
	potato := sha256.New()
	potato.Write(b)
	potato.Write(tb)
	potato.Write([]byte(destDir))
	newHandle := hex.EncodeToString(potato.Sum(nil)[:32])

	// use the same extension (so that a .jpg remains a .jpg etc.)
	chunks = strings.SplitN(file, ".", 2)
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

	hasVideo := false
	// also copy the adjuct, if there is one
	if img.Kind == MediaGenerated {
		adjunct := Repository.canonFile(filepath.Join(Repository.dirFor(camera, img.Kind), fmt.Sprintf("%s.webm", img.Handle)))
		_, err := os.Stat(adjunct)
		if err != nil {
			if os.IsNotExist(err) {
				panic(fmt.Errorf("generated image '%s' is missing adjunct at '%s' (%s)", img.Handle, adjunct, err))
			}
			panic(err)
		}
		hasVideo = true

		b, err := ioutil.ReadFile(adjunct)

		destFile := Repository.canonFile(filepath.Join(destDir, fmt.Sprintf("%s.webm", newHandle)))
		if err := ioutil.WriteFile(destFile, b, 0660); err != nil {
			panic(err)
		}
	}

	// return a handle to the new image
	return &Image{
		Handle:    newHandle,
		Source:    camera,
		Kind:      MediaPinned,
		Timestamp: img.Timestamp,
		HasVideo:  hasVideo,
		stat:      newStat,
		diskPath:  destFile,
	}
}

// LinkVideo associates video bytes with the image, which is understood to be a
// still frame from the video, suitable for use as a thumbnail or cover still
// for the video. Errors if the image is not a video type (i.e. generated/timelapse.)
func (img *Image) LinkVideo(content []byte) {
	if img.Kind != MediaGenerated {
		panic(fmt.Errorf("unable to associate video data with %s for '%s'", img.Kind, img.Handle))
	}
	fname := strings.Join([]string{img.Handle, "webm"}, ".")
	fname = Repository.canonFile(filepath.Join(Repository.dirFor(img.Source, img.Kind), fname))
	if _, err := os.Stat(fname); err != nil {
		if !os.IsNotExist(err) {
			log.Error("Image.LinkVideo", "image '%s' already has video", img.Handle)
			panic(err)
		}
	}
	if err := ioutil.WriteFile(fname, content, 0660); err != nil {
		panic(err)
	}
}

// RetrieveVideo fetches the bytes of the video for which the image is a still.
// Besides the usual errors, this will also error if the image is not a
// video-carrying Kind.
func (img *Image) RetrieveVideo(buf *bytes.Buffer) {
	fname := strings.Join([]string{img.Handle, "webm"}, ".")
	fname = Repository.canonFile(filepath.Join(Repository.dirFor(img.Source, img.Kind), fname))
	if b, err := ioutil.ReadFile(fname); err != nil {
		panic(err)
	} else {
		if n, err := buf.Write(b); err != nil {
			panic(err)
		} else if n != len(b) {
			panic("partial write to memory buffer")
		}
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

// dewarpFisheye implements distortion correction for a fisheye lens. This is
// currently hardcoded with parameter values suitable for the Wyze camera v2,
// but could be adapted. It is based on
// http://www.tannerhelland.com/4743/simple-algorithm-correcting-lens-distortion/
// but with added subpixel interpolation.
func dewarpFisheye(b []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}

	d := image.NewRGBA(img.Bounds())
	width := d.Bounds().Size().X
	height := d.Bounds().Size().Y
	halfY := height / 2
	halfX := width / 2

	strength := 2.35
	corrRad := math.Sqrt(float64(width*width+height*height)) / strength
	EPSILON := 0.0000000001
	zoom := 1.00

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			absX := float64(x - halfX)
			absY := float64(y - halfY)

			dist := math.Sqrt(absX*absX + absY*absY)
			r := dist / corrRad
			theta := 1.
			if r > EPSILON {
				theta = math.Atan(r) / r
			}

			srcX := float64(halfX) + theta*absX*zoom
			srcY := float64(halfY) + theta*absY*zoom

			// (srcX, srcY) will point to a place between pixels; interpolate its color value by weighting its neighbors'
			loX := int(srcX)
			hiX := loX + 1
			dX := srcX - float64(loX)

			loY := int(srcY)
			hiY := loY + 1
			dY := srcY - float64(loY)

			Ri, Gi, Bi, Ai := img.At(loX, loY).RGBA()
			R := float64(Ri) * (1 - dX) * (1 - dY)
			G := float64(Gi) * (1 - dX) * (1 - dY)
			B := float64(Bi) * (1 - dX) * (1 - dY)
			A := float64(Ai) * (1 - dX) * (1 - dY)

			Ri, Gi, Bi, Ai = img.At(hiX, loY).RGBA()
			R += float64(Ri) * dX * (1 - dY)
			G += float64(Gi) * dX * (1 - dY)
			B += float64(Bi) * dX * (1 - dY)
			A += float64(Ai) * dX * (1 - dY)

			Ri, Gi, Bi, Ai = img.At(loX, hiY).RGBA()
			R += float64(Ri) * (1 - dX) * dY
			G += float64(Gi) * (1 - dX) * dY
			B += float64(Bi) * (1 - dX) * dY
			A += float64(Ai) * (1 - dX) * dY

			Ri, Gi, Bi, Ai = img.At(hiX, hiY).RGBA()
			R += float64(Ri) * dX * dY
			G += float64(Gi) * dX * dY
			B += float64(Bi) * dX * dY
			A += float64(Ai) * dX * dY

			R16 := uint16(math.Round(R))
			G16 := uint16(math.Round(G))
			B16 := uint16(math.Round(B))
			A16 := uint16(math.Round(A))

			c := color.RGBA64{R: R16, G: G16, B: B16, A: A16}

			d.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	jpeg.Encode(&buf, d, nil)
	return buf.Bytes()
}
