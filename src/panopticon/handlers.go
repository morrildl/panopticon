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
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"time"

	"image"
	_ "image/gif" // register GIF support
	"image/jpeg"
	_ "image/png" // register PNG support

	"panopticon/messages"

	"playground/httputil"
	"playground/log"
	"playground/session"

	"github.com/bradfitz/latlong"
)

// userFor looks up the User instance, primarily for its Privileged flag for use
// in perms checks. This can return nil in principle, but we assume we're called
// with the emailInspector sentinel.
func userFor(req *http.Request) *User {
	ssn := session.GetSession(req)
	log.Debug("userFor", "session", ssn)
	log.Debug("userFor", "session email", ssn.Email)
	return System.GetUser(session.GetSession(req).Email)
}

// ProvisionHandler handles /provision
func ProvisionHandler(writer http.ResponseWriter, req *http.Request) {
	buf := &bytes.Buffer{}
	System.QR(buf)

	httputil.Send(writer, http.StatusOK, "image/png", buf.Bytes())
}

// StateHandler handles /state
func StateHandler(writer http.ResponseWriter, req *http.Request) {
	res := &messages.State{
		ServiceName:  System.ServiceName,
		DefaultImage: System.DefaultImage,
	}

	// no camera specified, load them all
	cameras := System.Cameras()
	u := userFor(req)

	now := time.Now()
	res.Cameras = []*messages.Camera{}
	for _, c := range cameras {
		if c.Private && !u.Privileged {
			continue
		}

		loc := time.UTC
		if tz := latlong.LookupZoneName(c.Latitude, c.Longitude); tz != "" {
			if computed, err := time.LoadLocation(tz); err == nil {
				loc = computed
			}
		}
		localNow := now.In(loc)

		mc := &messages.Camera{
			Name:        c.Name,
			ID:          c.ID,
			AspectRatio: c.AspectRatio,
			LocalTime:   localNow.Format("3:04pm"),
			LocalDate:   localNow.Format("Monday, 2 January, 2006"),
			Sleeping:    c.IsDark(),

			// currently unused fields
			Message: "",
			Offline: false,
		}

		var latest *Image

		recents, pinned, generated, motion := Repository.Recents(c.ID)
		if len(recents) < 1 {
			// technically wrong but if we have no recents it's very unlikely we have anything else
			res.Cameras = append(res.Cameras, mc)
			continue
		}

		// pop the first one off and use it as our hero image
		latest = recents[0]
		mc.LatestHandle = latest.Handle
		latestTS := latest.Timestamp.In(loc)
		mc.LatestTime = latestTS.Format("3:04pm")
		mc.LatestDate = latestTS.Format("Monday, 2 January, 2006")

		mc.Recent = []*messages.ImageMeta{}
		for i, img := range recents {
			if i == 0 {
				continue
			}
			mc.Recent = append(mc.Recent, &messages.ImageMeta{Handle: img.Handle, HasVideo: img.HasVideo})
		}

		mc.Saved = []*messages.ImageMeta{}
		for _, img := range pinned {
			mc.Saved = append(mc.Saved, &messages.ImageMeta{Handle: img.Handle, HasVideo: img.HasVideo})
		}

		mc.Timelapse = []*messages.ImageMeta{}
		for _, img := range generated {
			mc.Timelapse = append(mc.Timelapse, &messages.ImageMeta{Handle: img.Handle, HasVideo: img.HasVideo})
		}

		mc.Motion = []*messages.ImageMeta{}
		for _, img := range motion {
			mc.Motion = append(mc.Motion, &messages.ImageMeta{Handle: img.Handle, HasVideo: img.HasVideo})
		}

		res.Cameras = append(res.Cameras, mc)
	}

	httputil.SendJSON(writer, http.StatusOK, &APIResponse{Artifact: res})
}

// ImageMetaHandler handles /client/imagemeta
func ImageMetaHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.ImageMetaHandler"
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, missingImage)
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)
	ise := httputil.NewJSONAssertable(writer, TAG, http.StatusInternalServerError, internalError)

	imgID := httputil.ExtractSegment(req.URL.Path, 3)
	badReq.Assert(imgID != "" && imgID != "undefined", "bogus image ID '%s'", imgID)

	img := Repository.Locate(imgID)
	notFound.Assert(img != nil, "unknown image '%s'", imgID)

	u := userFor(req)
	camera := System.GetCamera(img.Source)
	ise.Assert(camera != nil, "image '%s' references unknown camera '%s'", img.Handle, img.Source)
	notFound.Assert(!camera.Private || u.Privileged, "attempt by '%s' to access private '%s'", u.Email, camera.ID)

	t := img.Timestamp
	loc := camera.Location()
	if loc != nil {
		t = t.In(loc)
	}
	res := &messages.ImageMeta{
		Handle:   img.Handle,
		Camera:   camera.Name,
		Time:     t.Format("3:04pm"),
		Date:     t.Format("Monday, 2 January, 2006"),
		HasVideo: img.HasVideo,
	}

	httputil.SendJSON(writer, http.StatusOK, &APIResponse{Artifact: res})
}

// ImageListHandler handles /client/images
func ImageListHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.ImageListHandler"
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, missingImage)
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)
	ise := httputil.NewJSONAssertable(writer, TAG, http.StatusInternalServerError, internalError)

	camera := httputil.ExtractSegment(req.URL.Path, 3)
	kindstr := httputil.ExtractSegment(req.URL.Path, 4)
	badReq.Assert(camera != "" && kindstr != "", "missing camera (%s) or kind (%s)", camera, kindstr)

	kind := Repository.segmentToMediaKind(kindstr)
	badReq.Assert(kind != MediaUnknown, "request for invalid kind '%s'", kindstr)

	u := userFor(req)
	cam := System.GetCamera(camera)
	notFound.Assert(cam != nil, "request for unknown camera '%s'", camera)
	notFound.Assert(!cam.Private || u.Privileged, "attempt by '%s' to access private '%s'", u.Email, cam.ID)

	imgs := Repository.ListKind(camera, kind)

	// reverse sort by timestamp
	sort.Slice(imgs, func(i, j int) bool { return imgs[i].Timestamp.After(imgs[j].Timestamp) })

	skip := 0
	per := 0

	err := req.ParseForm()
	ise.Assert(err == nil, "error parsing request form (%s)", err)

	raw := req.Form.Get("skip")
	if raw != "" {
		skip, err = strconv.Atoi(raw)
		badReq.Assert(err == nil, "unparseable skip value '%s' (%s)", raw, err)
	}
	raw = req.Form.Get("per")
	if raw != "" {
		per, err = strconv.Atoi(raw)
		badReq.Assert(err == nil, "unparseable per value '%s' (%s)", raw, err)
	}

	res := []*messages.ImageMeta{}

	if skip < len(imgs) {
		end := len(imgs)
		if skip+per < end {
			end = skip + per
		}
		loc := cam.Location()
		if loc == nil {
			loc = time.UTC
		}
		for _, img := range imgs[skip:end] {
			ts := img.Timestamp.In(loc)
			meta := &messages.ImageMeta{
				Camera:   cam.Name,
				Handle:   img.Handle,
				Time:     ts.Format("3:04pm"),
				Date:     ts.Format("Monday, 2 January, 2006"),
				HasVideo: img.HasVideo,
			}
			res = append(res, meta)
		}
	} // else we're past the end, so return no results

	httputil.SendJSON(writer, http.StatusOK, &APIResponse{Artifact: &messages.ImageList{Camera: cam.Name, Total: len(imgs), Images: res}})
}

// ImageHandler handles /client/image and /client/video
func ImageHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.ImageHandler"
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, missingImage)
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)

	ctype := "image/jpeg"
	var buf bytes.Buffer

	imgID := httputil.ExtractSegment(req.URL.Path, 3)
	if imgID == "" || imgID == "undefined" {
		if b, err := ioutil.ReadFile(Repository.DefaultImage); err == nil {
			if n, err := buf.Write(b); n != len(b) || err != nil {
				panic(err)
			}
		} else {
			panic(err)
		}
	} else {
		img := Repository.Locate(imgID)
		notFound.Assert(img != nil, "failed to locate a requested image '%s'", imgID)

		u := userFor(req)
		cam := System.GetCamera(img.Source)
		notFound.Assert(!cam.Private || u.Privileged, "attempt by '%s' to access private '%s'", u.Email, cam.ID)

		mode := httputil.ExtractSegment(req.URL.Path, 2)
		badReq.Assert(mode != "video" || img.HasVideo, "attempt to access video for non-video '%s'", img.Handle, *img)

		if mode == "video" {
			ctype = "video/x-msvideo"
			img.RetrieveVideo(&buf)
		} else {
			img.Retrieve(&buf)
		}
	}

	httputil.Send(writer, http.StatusOK, ctype, buf.Bytes())
}

// SaveHandler handles /client/save/
func SaveHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.SaveHandler"
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, noSuchCamera)

	imgID := httputil.ExtractSegment(req.URL.Path, 3)
	badReq.Assert(imgID != "", "missing image ID")

	img := Repository.Locate(imgID)
	notFound.Assert(img != nil, "unknown image '%s'", imgID)
	cam := System.GetCamera(img.Source)
	u := userFor(req)
	notFound.Assert(!cam.Private || u.Privileged, "attempt by '%s' to access private '%s'", u.Email, cam.ID)

	alreadyPinned := !img.Pin(MediaSaved)

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: struct {
		NewHandle string
		Duplicate bool
	}{img.Handle, alreadyPinned}})
}

// MotionHandler handles /camera/motion
func MotionHandler(writer http.ResponseWriter, req *http.Request) {
	processUpload(writer, req, MediaMotion)
}

// LatestHandler handles /camera/latest
func LatestHandler(writer http.ResponseWriter, req *http.Request) {
	processUpload(writer, req, MediaCollected)
}

func processUpload(writer http.ResponseWriter, req *http.Request, kind MediaKind) {
	TAG := "panopticon.processUpload"
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)
	ise := httputil.NewJSONAssertable(writer, TAG, http.StatusInternalServerError, internalError)
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, noSuchCamera)

	camID := req.Header.Get(System.CameraIDHeader)
	badReq.Assert(camID != "", "missing camera ID header")

	cam := System.GetCamera(camID)
	notFound.Assert(cam != nil, "attempt to upload by unknown '%s'", camID)

	b, err := ioutil.ReadAll(req.Body)
	ise.Assert(err == nil, "error loading request (%s)", err)
	img, _, err := image.Decode(bytes.NewReader(b))
	badReq.Assert(err == nil, "bytes uploaded are not an image (%s)", err)

	// check local sunrise/sunset times (w/ 15m window either direction) and don't bother to record night images
	// note that this isn't an error: cameras are assumed to be dumb and not implementing this behavior
	if cam.IsDark() {
		httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: struct{}{}})
		return
	}

	// convert to JPEG; could save CPU by not re-encoding if already JPEG, but might as well anyway for safety
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, nil)
	ise.Assert(err == nil, "error converting to JPEG (%s)", err)

	handle := Repository.Store(camID, buf.Bytes())
	handle.Pin(kind)
	res := &struct{ Handle, Timestamp string }{handle.Handle, handle.PrettyTime()}

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: res})
}
