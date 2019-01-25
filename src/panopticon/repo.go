package panopticon

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	//sun "github.com/kelvins/sunrisesunset"
	//"github.com/cpucycle/astrotime"

	"playground/log"

	sunrise "github.com/nathan-osman/go-sunrise"
)

// RepositoryConfig contains photos. Essentially it owns and oversees the directory where photos are stored.
type RepositoryConfig struct {
	BaseDirectory   string
	RetentionPeriod string
	Latitude        string
	Longitude       string
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

	repo.startTimelapser(0, 0)
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

	for _, kind := range []MediaKind{MediaCollected, MediaMotion} {
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

func scheduler(tag string, hour int, min int, job func()) {
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

// PurgeAt configures a job to purge the indicated kindo of image according to
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

	if kind != MediaCollected && kind != MediaMotion {
		panic(fmt.Errorf("cannot generate timelapse for '%s' content", kind))
	}

	if camera.Diurnal {
		if camera.Latitude == 0.0 && camera.Longitude == 0.0 {
			// unlikely someone has a camera at north pole...
			log.Warn(TAG, fmt.Sprintf("camera '%s' lacks lat or lng %f %f", camera.ID, camera.Latitude, camera.Longitude))
		} else {
			start, end = sunrise.SunriseSunset(camera.Latitude, camera.Longitude, date.Year(), date.Month(), date.Day())
			start = start.Local().Add(-15 * time.Minute)
			end = end.Local().Add(15 * time.Minute)
		}
	}

	var next time.Time
	images := repo.List(camera.ID)
	log.Debug(TAG, "images", len(images))
	candidates := []*Image{}
	for _, img := range images {
		if img.Kind != kind {
			continue
		}
		if img.Timestamp.Before(start) || end.Before(img.Timestamp) {
			continue
		}
		candidates = append(candidates, img)
	}
	log.Debug(TAG, "candidates", len(candidates))
	log.Debug(TAG, "times", start, end)
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
		names = append(names, img.diskPath)
	}

	// images now contains a sorted list of all files that should be in the timelapse
	log.Debug(TAG, "final images", len(images))
	if len(images) < 1 {
		log.Debug(TAG, "no images from which to generate timelapse")
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

	// Run it: mencoder "mf://@/path/to/index" -mf fps=24 -o /path/to/generated.avi -ovc x264
	args := "mf://@%s -mf fps=24 -o %s -ovc x264"
	avi := path.Join(dir, "generated.avi")
	args = fmt.Sprintf(args, indexPath, avi)
	cmd := exec.Command("mencoder", strings.Split(args, " ")...)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Remove(avi); err != nil {
			log.Error(TAG, fmt.Sprintf("failed to remove generated '%s'", avi), err)
		}
	}()
	log.Debug(TAG, "started mencoder with args", args)
	if err := cmd.Wait(); err != nil {
		panic(err)
	}
	log.Debug(TAG, fmt.Sprintf("mencoder complete for '%s'", avi))

	content, err := ioutil.ReadFile(avi)
	if err != nil {
		panic(err)
	}
	repo.Store(camera.ID, MediaGenerated, "avi", content)

	log.Status(TAG, fmt.Sprintf("generated timelapse for '%s' from %d images", camera.ID, len(images)))
}

func (repo *RepositoryConfig) startTimelapser(hour int, min int) {
	job := func() {
		for _, camera := range System.Cameras() {
			today := time.Now().Local()
			go func(camera *Camera) {
				if camera.Timelapse == MediaCollected || camera.Timelapse == "both" {
					repo.GenerateTimelapse(today, camera, MediaCollected)
				}
			}(camera)
			go func(camera *Camera) {
				if camera.Timelapse == MediaMotion || camera.Timelapse == "both" {
					repo.GenerateTimelapse(today, camera, MediaMotion)
				}
			}(camera)
		}
	}

	go scheduler("timelapser", hour, min, job)
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
