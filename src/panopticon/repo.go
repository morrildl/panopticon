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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"playground/log"
)

// RepositoryConfig contains photos. Essentially it owns and oversees the directory where photos are stored.
type RepositoryConfig struct {
	BaseDirectory   string
	RetentionPeriod string
	Latitude        string
	Longitude       string
	DefaultImage    string
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

	repo.PurgeAt(4, 0, "24h", MediaCollected)
	repo.PurgeAt(4, 15, "24h", MediaMotion)
	repo.PurgeAt(4, 30, repo.RetentionPeriod, MediaGenerated)
	repo.VacuumAt(4, 45)

	repo.startTimelapser(0, 0)
}

// Store updates the latest image for the given source (camera.) The provided image data will be
// stored to disk, and will replace the previous in-RAM copy as latest from that source. The
// `handle` returned can be used to refer to the image later, e.g. for lookups.
func (repo *RepositoryConfig) Store(source string, data []byte) *Image {
	return CreateImage(source, data)
}

// Latest retrieves the most recent image data received from the indicated source.
func (repo *RepositoryConfig) Latest(source string) *Image {
	var res *Image

	for _, kind := range []MediaKind{MediaCollected, MediaMotion} {
		for _, img := range repo.ListKind(source, kind) {
			if res == nil {
				res = img
				continue
			}
			if img.Timestamp.After(res.Timestamp) {
				res = img
			}
		}
	}

	return res
}

// Recents returns recent photo activity. It will return up to 7 most recent
// images (collected or motion), and up to 4 most recent of the others.
func (repo *RepositoryConfig) Recents(camera string) (recents []*Image, saved []*Image, generated []*Image, motion []*Image) {
	TAG := "RepositoryConfig.Recents"

	cam := System.GetCamera(camera)
	if cam == nil {
		panic(fmt.Errorf("unknown camera '%s'", camera))
	}

	for _, kind := range AllKinds {
		for _, img := range repo.ListKind(camera, kind) {
			switch kind {
			case MediaMotion:
				motion = append(motion, img)
				fallthrough
			case MediaCollected: // recents is a *mix* of collected + motion
				recents = append(recents, img)
			case MediaGenerated:
				generated = append(generated, img)
			case MediaSaved:
				saved = append(saved, img)
			default:
				log.Warn(TAG, "unknown media kind?!", kind)
			}
		}
	}

	// note that these need to be sorted in descending order by date, so the comparator is backward
	sort.Slice(recents, func(i, j int) bool { return recents[i].Timestamp.After(recents[j].Timestamp) })
	sort.Slice(saved, func(i, j int) bool { return saved[i].Timestamp.After(saved[j].Timestamp) })
	sort.Slice(generated, func(i, j int) bool { return generated[i].Timestamp.After(generated[j].Timestamp) })
	sort.Slice(motion, func(i, j int) bool { return motion[i].Timestamp.After(motion[j].Timestamp) })

	if len(recents) > 7 {
		recents = recents[:7]
	}
	if len(saved) > 4 {
		saved = saved[:4]
	}
	if len(motion) > 4 {
		motion = motion[:4]
	}
	if len(generated) > 4 {
		generated = generated[:4]
	}

	return
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
				name := entry.Name()
				if kind == MediaGenerated && strings.Split(name, ".")[1] != "jpg" {
					continue
				}
				if strings.HasPrefix(name, handle) {
					_, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%s.webm", handle)))
					hasVideo := (kind == MediaGenerated || err == nil)
					return &Image{
						Handle:    handle,
						Source:    camera.ID,
						Timestamp: entry.ModTime(),
						HasVideo:  hasVideo,
					}
				}
			}
		}
	}

	return nil
}

// ListKind returns the handles of all images of the indicated kind, associated with the indicated source.
func (repo *RepositoryConfig) ListKind(source string, kind MediaKind) []*Image {
	if System.GetCamera(source) == nil {
		panic(fmt.Errorf("attempt to list unknown camera '%s'", source))
	}

	images := []*Image{}
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
		s := strings.Split(entry.Name(), ".")
		if s[1] != "jpg" {
			// .webm files live alonside their .jpg still images, but we shouldn't return them as handles`
			continue
		}
		_, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%s.webm", s[0])))
		hasVideo := (kind == MediaGenerated || err == nil)
		images = append(images, &Image{
			Handle:    s[0],
			Source:    source,
			Timestamp: entry.ModTime(),
			HasVideo:  hasVideo,
		})
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

func scheduler(tag string, hour int, min int, job func()) {
	for {
		now := time.Now().Local()

		// compute how long we need to sleep for
		goal := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.Local)
		if goal.Before(now) {
			// specified time already happened today, so advance to same time tomorrow
			goal = goal.Add(24 * time.Hour)
		}
		delta := goal.Sub(now)

		log.Debug(tag, fmt.Sprintf("sleeping for %s until %s", delta/time.Nanosecond, goal.Format(time.RFC3339)))
		time.Sleep(delta)

		log.Debug(tag, "running as configured")
		job()
	}
}

// PurgeAt configures a job to purge the indicated kind of image according to
// the indicated retention period to run each day at the indicated time.
func (repo *RepositoryConfig) PurgeAt(hour int, min int, retention string, kind MediaKind) {
	dur, err := time.ParseDuration(retention) // e.g. "14d", "24h"
	if err != nil {
		panic(err)
	}

	// a tiny function to encapsulate what we need to do when we reach our start time
	job := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("RepositoryConfig.Purge.job", "panic in purge job function", r)
			}
		}()

		when := time.Now().Add(-dur)
		repo.PurgeBefore(kind, when)
	}

	go scheduler("purger", hour, min, job)
}

// GC deletes all leaf image files that are not pinned, i.e. it garbage collects.
func (repo *RepositoryConfig) GC() {
	TAG := "RepositoryConfig.Vacuum"
	allImages := make(map[string]bool)
	for _, cam := range System.Cameras() {
		for _, kind := range AllKinds {
			for _, img := range repo.ListKind(cam.ID, kind) {
				allImages[img.Handle] = true
			}
		}
	}

	// loop over all raw disk files
	shaRE := regexp.MustCompile("[a-fA-F0-9]{64}")
	for _, cam := range System.Cameras() {
		baseDir := repo.canonDir(filepath.Join(repo.BaseDirectory, cam.ID, MediaData))
		base, err := os.Open(baseDir)
		if err != nil {
			panic(err)
		}
		defer base.Close()
		entries, err := base.Readdir(0)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			// these are the parent directories for our SHA2-256-hashed image/video files
			if !entry.IsDir() {
				continue
			}
			if len(entry.Name()) != 3 {
				// not one of our 3-character summary dirs
				continue
			}
			subdir := repo.canonDir(filepath.Join(baseDir, entry.Name()))
			sd, err := os.Open(subdir)
			if err != nil {
				panic(err)
			}
			defer sd.Close()
			leaves, err := sd.Readdir(0)
			if err != nil {
				panic(err)
			}
			for _, leaf := range leaves {
				// these should be our actual media files
				if leaf.IsDir() {
					continue
				}
				chunks := strings.Split(leaf.Name(), ".")
				if len(chunks) != 3 {
					// not a file we know how to deal with
					continue
				}
				handle := chunks[0]
				if !shaRE.Match([]byte(handle)) {
					// not one of our sha256 filenames
					log.Debug(TAG, fmt.Sprintf("skipping unrecognized file '%s'", leaf.Name()))
					continue
				}
				if _, ok := allImages[handle]; ok {
					// means it's pinned by one of the other MediaKinds
					log.Debug(TAG, fmt.Sprintf("skipping pinned file '%s'", leaf.Name()))
					continue
				}

				// final sanity check, and then remove it
				fpath := filepath.Join(subdir, leaf.Name())
				canonPath := repo.canonFile(fpath)
				if fpath != canonPath {
					log.Error(TAG, fmt.Sprintf("noncanonical path set '%s'/'%s'", fpath, canonPath))
					continue
				}
				err := os.Remove(canonPath)
				if err != nil {
					panic(err)
				}
				log.Debug(TAG, fmt.Sprintf("removed unpinned file '%s'", canonPath))
			}
		}
	}
}

// VacuumAt configures a job to vacuum/GC raw image files that are not pinned.
func (repo *RepositoryConfig) VacuumAt(hour int, min int) {
	job := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("RepositoryConfig.Vacuum.job", "panic in vacuum job function", r)
			}
		}()

		repo.GC()
	}

	// start a background thread that runs forever, waking up every minute to compare time
	go scheduler("purger", hour, min, job)
}

// GenerateTimelapse scans for all files matching the indicated source and kind
// taken during the indicated date, and generates a timelapse from them. If
// `diurnal` is true, it uses astronomical sunrise & sunset to limit the
// timelapse to only daylight hours.
func (repo *RepositoryConfig) GenerateTimelapse(date time.Time, camera *Camera, kind MediaKind) {
	TAG := "RepositoryConfig.GenerateTimelapse"

	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, date.Location())
	log.Debug(TAG, "date range", date, start, end)

	if kind != MediaCollected && kind != MediaMotion {
		panic(fmt.Errorf("cannot generate timelapse for '%s' content", kind))
	}

	if camera.Diurnal {
		_, start, end = camera.LocalDaylight(date)
	}

	var next time.Time
	images := repo.ListKind(camera.ID, kind)
	candidates := []*Image{}
	for _, img := range images {
		if img.Timestamp.Before(start) || end.Before(img.Timestamp) {
			continue
		}
		candidates = append(candidates, img)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Timestamp.Before(candidates[j].Timestamp) })
	images = []*Image{}
	names := []string{}
	for _, img := range candidates {
		if img.Timestamp.Before(next) {
			continue
		} else {
			next = img.Timestamp.Add(29 * time.Second) // no more than 2 frames per minute
		}
		images = append(images, img)
		dataPath := repo.dataPath(img.Source, fmt.Sprintf("%s.jpg", img.Handle))
		names = append(names, dataPath)
	}

	// images now contains a sorted list of all files that should be in the timelapse
	if len(images) < 1 {
		log.Warn(TAG, "no images from which to generate timelapse")
		return
	}

	// create a temp dir to generate our timelapse in via shelling out to mencoder
	dir, err := ioutil.TempDir("/tmp", "timelapse-")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Remove(dir); err != nil {
			log.Error(TAG, fmt.Sprintf("failed to remove tempdir '%s'", dir), err)
		}
	}()

	// create a file listing other files to include in the timelapse, to be passed to mencoder
	indexPath := path.Join(dir, "index")
	if err := ioutil.WriteFile(indexPath, []byte(strings.Join(names, "\n")), 0600); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Remove(indexPath); err != nil {
			log.Error(TAG, fmt.Sprintf("failed to remove index '%s'", indexPath), err)
		}
	}()

	args := "mf://@%s -mf fps=24 -o %s -of lavf -ovc lavc -lavfopts format=webm -lavcopts threads=8:vcodec=libvpx -ffourcc VP80"
	webm := path.Join(dir, "generated.webm")
	args = fmt.Sprintf(args, indexPath, webm)
	cmd := exec.Command("mencoder", strings.Split(args, " ")...)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Remove(webm); err != nil {
			log.Error(TAG, fmt.Sprintf("failed to remove generated '%s'", webm), err)
		}
	}()
	log.Debug(TAG, fmt.Sprintf("started mencoder for '%s' with args", camera.ID), args)
	if err := cmd.Wait(); err != nil {
		panic(err)
	}
	log.Debug(TAG, fmt.Sprintf("mencoder complete for '%s'", webm))

	webmBytes, err := ioutil.ReadFile(webm)
	if err != nil {
		panic(err)
	}

	still := images[len(images)/2] // already checked len(images) > 0
	var buf bytes.Buffer
	still.Retrieve(&buf)
	stillBytes := buf.Bytes()

	img := repo.Store(camera.ID, stillBytes)
	if img == nil {
		log.Warn(TAG, fmt.Sprintf("nonerror result but nil image"))
	} else {
		img.LinkVideo(webmBytes)
	}
	img.Pin(MediaGenerated)

	log.Status(TAG, fmt.Sprintf("generated timelapse for '%s' from %d images", camera.ID, len(images)))
}

func (repo *RepositoryConfig) startTimelapser(hour int, min int) {
	job := func() {
		for _, camera := range System.Cameras() {
			today := time.Now().Local().Add(-24 * time.Hour) // do yesterday's timelapse
			go func(camera *Camera) {
				defer func() {
					if r := recover(); r != nil {
						log.Error("timelapse", fmt.Sprintf("error generating timelapse for '%s'", camera.ID), r)
					}
				}()
				if camera.Timelapse == MediaCollected || camera.Timelapse == "both" {
					repo.GenerateTimelapse(today, camera, MediaCollected)
				}
			}(camera)
			go func(camera *Camera) {
				defer func() {
					if r := recover(); r != nil {
						log.Error("timelapse", fmt.Sprintf("error generating timelapse for '%s'", camera.ID), r)
					}
				}()
				if camera.Timelapse == MediaMotion || camera.Timelapse == "both" {
					repo.GenerateTimelapse(today, camera, MediaMotion)
				}
			}(camera)
		}
	}

	go scheduler("timelapser", hour, min, job)
}

/*
 * Directory Structure
 *
 * The base media directory for a given camera is
 * {{.BaseDirectory}}/{{.CameraID}}/{{.MediaData}} -- that is, all images
 * received from that camera are stored in this tree.
 *
 * Images are content-addressed, via their SHA2-256 hashes. However to help
 * avoid filesystem directory-entry limits, they are grouped in intermediate
 * directories based on the first 3 characters of their hash.
 *
 * So for example, a file received from `dachacam` might be stored as:
 * /path/to/images/dachacam/data/fee/feedfacedeadbeefcafebabef00db0a71337b00bc001b0a7.jpg
 *
 * Once so stored, these images must be pinned or they'll be reclaimed by the
 * next GC run. An image may be pinned in one or more other media kinds.
 * Pinning is a symlink from a kind directory to the data directory, but with
 * no intermediate grouping directory.
 *
 * For instance the data file above could be marked as pinned as a
 * MediaCollected via this creation of this file pointing to it as a symlink:
 * /path/to/images/dachacam/collected/feedfacedeadbeefcafebabef00db0a71337b00bc001b0a7.jpg
 *
 * Collected images are purged every 24 hours, via the removal of the symlink.
 *
 * Motion images are pushed in response to camera-side motion detection. They
 * are purged every 24 hours.
 *
 * Generated media -- basically timelapses -- are constructed daily per some
 * schedule. Only photos from collected and motion sets are eligible to be used
 * to generate images. Generated images are purged after 14 days.
 *
 * Saved images are images flagged by the user for permanent retention. They
 * are, obviously, never purged.
 *
 * As above, any base data file not pinned via one of the other types is purged
 * on the next GC run.
 */

func (repo *RepositoryConfig) segmentToMediaKind(segment string) MediaKind {
	kind, ok := map[string]MediaKind{
		"collected": MediaCollected,
		"motion":    MediaMotion,
		"pinned":    MediaSaved,
		"generated": MediaGenerated,
	}[segment]
	if !ok {
		return MediaUnknown
	}
	return kind
}

func (repo *RepositoryConfig) dataPath(source, filename string) string {
	if len(filename) < 3 {
		panic("filename is too short")
	}
	prefix := filename[:3]
	dirPath := filepath.Join(repo.BaseDirectory, source, MediaData, prefix)
	repo.assertDir(dirPath)
	return filepath.Join(dirPath, filename)
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
