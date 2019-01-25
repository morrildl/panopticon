package panopticon

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"image"
	_ "image/gif" // register GIF support
	"image/jpeg"
	_ "image/png" // register PNG support

	"panopticon/messages"

	"playground/httputil"

	"github.com/bradfitz/latlong"
)

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

	now := time.Now()
	res.Cameras = []*messages.Camera{}
	for _, c := range cameras {
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

		mc.RecentHandles = []string{}
		for i, img := range recents {
			if i == 0 {
				continue
			}
			mc.RecentHandles = append(mc.RecentHandles, img.Handle)
		}

		mc.PinnedHandles = []string{}
		for _, img := range pinned {
			mc.PinnedHandles = append(mc.PinnedHandles, img.Handle)
		}

		mc.TimelapseHandles = []string{}
		for _, img := range generated {
			mc.TimelapseHandles = append(mc.TimelapseHandles, img.Handle)
		}

		mc.MotionHandles = []string{}
		for _, img := range motion {
			mc.MotionHandles = append(mc.MotionHandles, img.Handle)
		}

		res.Cameras = append(res.Cameras, mc)
	}

	httputil.SendJSON(writer, http.StatusOK, &APIResponse{Artifact: res})
}

// ImageHandler handles /client/image
func ImageHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.ImageHandler"
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, missingImage)

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
		handle := Repository.Locate(imgID)
		notFound.Assert(handle != nil, "failed to locate a requested image '%s'", imgID)

		handle.Retrieve(&buf)
	}

	httputil.Send(writer, http.StatusOK, "image/jpeg", buf.Bytes())
}

// PinHandler handles /client/pin/
func PinHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.PinHandler"
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, clientError)
	ise := httputil.NewJSONAssertable(writer, TAG, http.StatusInternalServerError, internalError)
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, noSuchCamera)

	imgID := httputil.ExtractSegment(req.URL.Path, 3)
	badReq.Assert(imgID != "", "missing image ID")

	img := Repository.Locate(imgID)
	notFound.Assert(img != nil, "unknown image '%s'", imgID)

	pinned := img.Pin()
	ise.Assert(pinned != nil, "nil result from pin operation on '%s'", imgID)

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: struct{ NewHandle string }{pinned.Handle}})
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

	handle := Repository.Store(camID, kind, "jpg", buf.Bytes())
	res := &struct{ Handle, Timestamp string }{handle.Handle, handle.PrettyTime()}

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: res})
}
