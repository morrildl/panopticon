package panopticon

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"image"
	_ "image/gif" // register GIF support
	"image/jpeg"
	_ "image/png" // register PNG support

	"panopticon/messages"

	"playground/httputil"
	"playground/log"

	sunrise "github.com/nathan-osman/go-sunrise"
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
		ServiceName: System.ServiceName,
	}

	// no camera specified, load them all
	cameras := System.Cameras()

	now := time.Now()
	res.Cameras = []*messages.Camera{}
	for _, c := range cameras {
		latest := Repository.Latest(c.ID)

		var latestURL string
		if latest == nil {
			latestURL = fmt.Sprintf("/static/no-image.png")
		} else {
			latestURL = fmt.Sprintf("/client/image/%s", latest.Handle)
		}

		mc := &messages.Camera{
			Name:           c.Name,
			ID:             c.ID,
			LatestImageURL: latestURL,
			LocalTime:      now.Format("3:04pm"), // TODO: timezones
			LocalDate:      now.Format("Monday, 2 January, 2006"),
			/* TODO: compute & fill in
			Message: "",
			*/
		}
		res.Cameras = append(res.Cameras, mc)
	}

	httputil.SendJSON(writer, http.StatusOK, &APIResponse{Artifact: res})
}

// ImageHandler handles /client/image
func ImageHandler(writer http.ResponseWriter, req *http.Request) {
	TAG := "panopticon.ImageHandler"
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, appError)
	notFound := httputil.NewJSONAssertable(writer, TAG, http.StatusNotFound, missingImage)

	imgID := httputil.ExtractSegment(req.URL.Path, 3)
	badReq.Assert(imgID != "", "missing image ID")

	handle := Repository.Locate(imgID)
	notFound.Assert(handle != nil, "failed to locate a requested image '%s'", imgID)

	var buf bytes.Buffer
	handle.Retrieve(&buf)

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
	img, imgType, err := image.Decode(bytes.NewReader(b))
	badReq.Assert(err == nil, "bytes uploaded are not an image (%s)", err)

	// check local sunrise/sunset times (w/ 15m window either direction) and don't bother to record night images
	// note that this isn't an error: cameras are assumed to be dumb and not implementing this behavior
	if cam.Diurnal {
		now := time.Now().Local()
		if cam.Latitude == 0.0 && cam.Longitude == 0.0 {
			// unlikely someone has a camera at north pole...
			log.Warn(TAG, fmt.Sprintf("diurnal camera '%s' lacks lat or lng %f %f", cam.ID, cam.Latitude, cam.Longitude))
			// note: no early return -- this will carry on below to record the photo
		} else {
			rise, set := sunrise.SunriseSunset(cam.Latitude, cam.Longitude, now.Year(), now.Month(), now.Day())
			rise = rise.Local().Add(-15 * time.Minute)
			set = set.Local().Add(15 * time.Minute)
			if now.Before(rise) || now.After(set) {
				httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: struct{}{}})
				return
			}
		}
	}

	// convert to JPEG if not already
	// TODO: check jpegginess
	log.Debug(TAG, "image type", imgType)
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, nil)
	ise.Assert(err == nil, "error converting to JPEG (%s)", err)

	handle := Repository.Store(camID, kind, "jpg", buf.Bytes())
	res := &struct{ Handle, Timestamp string }{handle.Handle, handle.PrettyTime()}

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: res})
}
