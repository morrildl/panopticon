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
	HasVideo  bool
}

// CreateImage stores the bytes to the disk according to config & convention, and returns a handle to the
// resulting image.
func CreateImage(source string, b []byte) *Image {
	cam := System.GetCamera(source)
	if cam.Dewarp {
		b = dewarpFisheye(b)
	}

	// image's stable ID is its hash
	potato := sha256.New()
	potato.Write(b)
	handle := hex.EncodeToString(potato.Sum(nil)[:32])

	// verify that the file doesn't somehow already exist
	diskPath := Repository.dataPath(source, fmt.Sprintf("%s.%s", handle, "jpg"))
	fi, err := os.Stat(diskPath)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	}
	if fi != nil {
		log.Error("Image.CreateImage", "file hash collision?! '%s'", handle)
	} else {
		// write the actual file contents under its computed name
		if err := ioutil.WriteFile(diskPath, b, 0660); err != nil {
			panic(err)
		}
	}

	stat, err := os.Stat(diskPath)
	if err != nil {
		panic(err)
	}

	return &Image{
		Handle:    handle,
		Source:    source,
		Timestamp: stat.ModTime(),
		HasVideo:  false,
	}
}

// LinkVideo associates video bytes with the image, which is understood to be a
// still frame from the video, suitable for use as a thumbnail or cover still
// for the video. Errors if the image is not a video type (i.e. generated/timelapse.)
func (img *Image) LinkVideo(content []byte) {
	basename := fmt.Sprintf("%s.%s", img.Handle, "webm")
	dataPath := Repository.dataPath(img.Source, basename)
	if fi, err := os.Stat(dataPath); err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	} else {
		if fi != nil {
			log.Error("Image.LinkVideo", "image '%s' already has video", img.Handle)
		}
	}
	if err := ioutil.WriteFile(dataPath, content, 0660); err != nil {
		panic(err)
	}
}

// Pin sets the Image to be pinned. `kind` must be one of the `Media*` enum constants. This is a
// link operation, not a copy or move. As most Kinds are subject to periodic purging, it is not
// necessary to unpin them; actual bytes are kept around as long as the image is
// pinned as at least one Kind.
//
// Images are expected to be created and media (if any) linked, before pinning.
// Pinning before linking video will result in the video media not being pinned
// or retained (but the image will be.)
//
// Returns true if the item was pinned, or false if it was already pinned.
func (img *Image) Pin(kind MediaKind) bool {
	basename := fmt.Sprintf("%s.%s", img.Handle, "jpg")
	dataPath := Repository.dataPath(img.Source, basename)
	if dataPath == "" {
		panic(fmt.Errorf("missing dataPath for image '%s'", img.Handle))
	}

	// extract file name, kind, and camera/source using our convention
	chunks := strings.Split(dataPath, string(os.PathSeparator))
	if len(chunks) < 4 {
		panic(fmt.Errorf("impossible disk path '%s'", dataPath))
	}
	file := chunks[len(chunks)-1]
	prefix := chunks[len(chunks)-2]
	diskKind := chunks[len(chunks)-3]
	camera := chunks[len(chunks)-4]
	if file == "" || prefix == "" || diskKind != MediaData || camera == "" || camera != img.Source {
		panic(fmt.Errorf("missing file/kind/camera '%s'/'%s'/'%s'", file, kind, camera))
	}

	// compute the new disk location; start with directory
	destDir := Repository.dirFor(camera, kind)

	// use the same extension (so that a .jpg remains a .jpg etc.)
	chunks = strings.SplitN(file, ".", 2)
	if len(chunks) != 2 {
		panic(fmt.Errorf("unable to decompose filename '%s'", file))
	}

	// now we can construct the fully qualified destination filename, and link it
	destFile := Repository.canonFile(filepath.Join(destDir, fmt.Sprintf("%s.%s", img.Handle, chunks[1])))
	fi, err := os.Stat(destFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	}
	if fi == nil {
		if err := os.Symlink(dataPath, destFile); err != nil {
			panic(err)
		}
	} else {
		log.Debug("Image.Pin", "double pin of '%s' to '%s'", img.Handle, kind)
		return false
	}

	// also link the video adjunct, if there is one
	basename = fmt.Sprintf("%s.%s", img.Handle, "webm")
	dataPath = Repository.dataPath(img.Source, basename)
	fi, err = os.Stat(dataPath)
	if err != nil && !os.IsNotExist(err) {
		panic(err)
	}
	if !os.IsNotExist(err) && fi != nil {

	}
	destFile = Repository.canonFile(filepath.Join(destDir, basename))
	if err := os.Symlink(dataPath, destFile); err != nil {
		panic(err)
	}
	return true
}

// Retrieve fetches the bytes for this image and stores them in the provided buffer.
func (img *Image) Retrieve(buf *bytes.Buffer) {
	diskPath := Repository.dataPath(img.Source, fmt.Sprintf("%s.%s", img.Handle, "jpg"))
	if diskPath == "" {
		panic(fmt.Errorf("missing diskPath for image '%s'", img.Handle))
	}
	b, err := ioutil.ReadFile(diskPath)
	if err != nil {
		panic(err)
	}
	n, err := buf.Write(b)
	if n != len(b) {
		panic(fmt.Errorf("partial write to memory buffer"))
	}
}

// RetrieveVideo fetches the bytes of the video for which the image is a still.
// Besides the usual errors, this will also error if the image is not a
// video-carrying Kind.
func (img *Image) RetrieveVideo(buf *bytes.Buffer) {
	fname := strings.Join([]string{img.Handle, "webm"}, ".")
	fname = Repository.dataPath(img.Source, fname)
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
