package panopticon

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image/png"
	"sort"
	"strconv"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

// TimelapseKind represents the different scenarios for which this system can generate a timelapse
// of images.
type TimelapseKind string

// constant values for TimelapseKind
const (
	TimelapseNone     TimelapseKind = ""
	TimelapseMotion                 = "motion"
	TimelapseDaylight               = "daylight"
)

// Camera represents a device permitted to upload images to this service. It also stores metadata
// about that camera, such as whether it's daylight-only, whether timelapses should be generated for
// it, etc.
type Camera struct {
	Name        string
	ID          string
	AspectRatio string
	Address     string
	Diurnal     bool
	Timelapse   TimelapseKind
	StillURL    string
	RTSPURL     string
}

// Store records a new Camera to the database, or updates it if it already exists.
func (c *Camera) Store() {
	cxn := System.getDB()
	defer cxn.Close()

	q := `insert into Cameras 
						(ID, Name, AspectRatio, Address, Diurnal, Timelapse, ImageURL, RTSPURL) 
						values (?, ?, ?, ?, ?, ?, ?)
						on conflict(ID) do update set
							Name=excluded.Name, AspectRatio=excluded.AspectRatio, Address=excluded.Address, Diurnal=excluded.Diurnal, 
							Timelapse=excluded.Timelapse, ImageURL=excluded.ImageURL, RTSPURL=excluded.RTSPURL`
	diurnal := 0
	if c.Diurnal {
		diurnal = 1
	}
	if _, err := cxn.Exec(q, c.ID, c.Name, c.Address, diurnal, c.Timelapse, c.StillURL, c.RTSPURL); err != nil {
		panic(err)
	}
}

// Delete removes a Camera from the database (revoking permissions for it and ultimately removing it
// from the UI.)
func (c *Camera) Delete() {
	cxn := System.getDB()
	defer cxn.Close()

	if _, err := cxn.Exec("delete from Cameras where ID=?", c.ID); err != nil {
		panic(err)
	}
}

// User represents an email (specifically, Google/Gmail) account that is permitted to access this
// system via OAuth2. It also records a meatspace name for that user.
type User struct {
	Email string
	Name  string
}

// Store records a new User to the database, or updates it if it already exists.
func (u *User) Store() {
	cxn := System.getDB()
	defer cxn.Close()

	if _, err := cxn.Exec("insert into Users (Email, Name) values (?, ?) on conflict(email) do update set Name=excluded.Name", u.Email, u.Name); err != nil {
		panic(err)
	}
}

// Delete removes a User from the database (revoking permissions to the web UI.)
func (u *User) Delete() {
	cxn := System.getDB()
	defer cxn.Close()

	if _, err := cxn.Exec("delete from Users where Email=?", u.Email); err != nil {
		panic(err)
	}
}

// SystemConfig abstracts the configuration database and also provides a central point for accessing
// various runtime settings.
type SystemConfig struct {
	HomeURL         string
	ServiceName     string
	SessionCookieID string
	CameraIDHeader  string
	RetentionPeriod string
	PollInterval    int
	SqlitePath      string
}

// Ready prepares the instance for use, generally by bootstrapping config from its sqlite3 database.
// Any values on the instance will be overwritten by the database, meaning the only field strictly
// required for initialization is the sqlite3 file path.
func (sys *SystemConfig) Ready() {
	sys.initSchema()

	cxn := sys.getDB()
	defer cxn.Close()

	// load settings table and overwrite anything we got from config with whatever is in the table
	if rows, err := cxn.Query("select Key, Value from Settings"); err != nil {
		panic(err)
	} else {
		defer rows.Close()

		var k, v string
		for rows.Next() {
			rows.Scan(&k, &v)
			if v == "" {
				continue
			}
			victim, ok := map[string]*string{
				"HomeURL":         &sys.HomeURL,
				"ServiceName":     &sys.ServiceName,
				"SessionCookieID": &sys.SessionCookieID,
				"CameraIDHeader":  &sys.CameraIDHeader,
				"RetentionPeriod": &sys.RetentionPeriod,
				// specifically exclude SqlitePath here
			}[k]
			if ok {
				*victim = v
			} else {
				victim, ok := map[string]*int{
					"PollInterval": &sys.PollInterval,
				}[k]
				if ok {
					if intV, err := strconv.Atoi(v); err != nil {
						panic(err) // shouldn't be listed as an int field but not have an int value
					} else {
						*victim = intV
					}
				}
			}
		}
	}
}

// QR generates a `data:` URL encoding a PNG image of a QR code that itself encodes the various
// system settings needed by a client to access this service. That is, this will generate a QR code
// that a device app can scan to populate itself with the settings necessary to interact with this
// service instance.
func (sys *SystemConfig) QR(buf *bytes.Buffer) {
	jsonBytes, err := json.Marshal(sys)
	if err != nil {
		panic(err)
	}
	qrImg, err := qr.Encode(string(jsonBytes), qr.M, qr.Auto)
	if err != nil {
		panic(err)
	}
	qrImg, err = barcode.Scale(qrImg, 200, 200)
	if err != nil {
		panic(err)
	}
	err = png.Encode(buf, qrImg)
	if err != nil {
		panic(err)
	}
}

// Cameras returns a list of all Camera rows currently configured. If there are no cameras, returns
// a nil slice.
func (sys *SystemConfig) Cameras() []*Camera {
	cxn := sys.getDB()
	defer cxn.Close()

	if rows, err := cxn.Query("select Name, ID, AspectRatio, Address, Diurnal, Timelapse, ImageURL, RTSPURL from Cameras"); err != nil {
		panic(err)
	} else {
		defer rows.Close()

		ret := []*Camera{}
		for rows.Next() {
			c := &Camera{}
			rows.Scan(&c.Name, &c.ID, &c.AspectRatio, &c.Address, &c.Diurnal, &c.Timelapse, &c.StillURL, &c.RTSPURL)
			if c.Name == "" || c.ID == "" {
				panic(fmt.Errorf("camera entry stored with null fields '%s'/'%s'", c.ID, c.Name))
			}
			ret = append(ret, c)
		}

		sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
		return ret
	}
}

// Users returns a list of all User rows currently configured. If there are no users, returns a nil
// slice.
func (sys *SystemConfig) Users() []*User {
	cxn := sys.getDB()
	defer cxn.Close()

	if rows, err := cxn.Query("select Email, Name from Users"); err != nil {
		panic(err)
	} else {
		defer rows.Close()

		ret := []*User{}
		for rows.Next() {
			u := &User{}
			rows.Scan(&u.Email, &u.Name)
			if u.Email == "" || u.Name == "" {
				panic(fmt.Errorf("user entry stored with null fields '%s'/'%s'", u.Email, u.Name))
			}
			ret = append(ret, u)
		}

		sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
		return ret
	}
}

// GetCamera fetches a specific Camera instance. If it returns a nil pointer but no error, that
// means there is no Camera associated with the provided ID.
func (sys *SystemConfig) GetCamera(ID string) *Camera {
	cxn := sys.getDB()
	defer cxn.Close()

	row := cxn.QueryRow("select Name, ID, AspectRatio, Address, Diurnal, Timelapse, ImageURL, RTSPURL from Cameras where ID=?", ID)

	c := &Camera{}
	err := row.Scan(&c.Name, &c.ID, &c.AspectRatio, &c.Address, &c.Diurnal, &c.Timelapse, &c.StillURL, &c.RTSPURL)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return c
}

// GetUser fetches a specific User instance. If it returns a nil pointer but no error, that means
// there is no User associated with the provided email.
func (sys *SystemConfig) GetUser(email string) *User {
	cxn := sys.getDB()
	defer cxn.Close()

	row := cxn.QueryRow("select Email, Name from Users where Email=?", email)

	u := &User{}
	err := row.Scan(&u.Email, &u.Name)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return u
}
