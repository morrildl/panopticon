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
)

// ProvisionHandler handles /provision
func ProvisionHandler(writer http.ResponseWriter, req *http.Request) {
	buf := &bytes.Buffer{}
	System.QR(buf)

	httputil.Send(writer, http.StatusOK, "image/png", buf.Bytes())
}

// StateHandler handles /state
func StateHandler(writer http.ResponseWriter, req *http.Request) {
	res := &messages.State{}

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

// ImageHandler handles /image
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

// MotionHandler handles /motion
func MotionHandler(writer http.ResponseWriter, req *http.Request) {
	processUpload(writer, req, PinMotion)
}

// LatestHandler handles /latest
func LatestHandler(writer http.ResponseWriter, req *http.Request) {
	processUpload(writer, req, PinNone)
}

func processUpload(writer http.ResponseWriter, req *http.Request, pinReason PinKind) {
	TAG := "panopticon.processUpload"
	badReq := httputil.NewJSONAssertable(writer, TAG, http.StatusBadRequest, appError)
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

	// convert to JPEG if not already
	// TODO: check jpegginess
	log.Debug(TAG, "image type", imgType)
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, nil)
	ise.Assert(err == nil, "error converting to JPEG (%s)", err)

	handle := Repository.Store(camID, pinReason, buf.Bytes())
	res := &struct{ Handle, Timestamp string }{handle.Handle, handle.PrettyTime()}

	httputil.SendJSON(writer, http.StatusAccepted, &APIResponse{Artifact: res})
}
