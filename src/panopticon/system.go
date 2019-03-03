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
	"database/sql"
	"encoding/json"
	"fmt"
	"image/png"
	"sort"
	"strconv"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/bradfitz/latlong"
	sunrise "github.com/nathan-osman/go-sunrise"
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
	Dewarp      bool
	Timelapse   MediaKind
	StillURL    string
	RTSPURL     string
	Latitude    float64
	Longitude   float64
	Private     bool
}

// Store records a new Camera to the database, or updates it if it already exists.
func (c *Camera) Store() {
	cxn := System.getDB()
	defer cxn.Close()

	q := `insert into Cameras 
						(ID, Name, AspectRatio, Address, Diurnal, Dewarp, Latitude, Longitude, Timelapse, ImageURL, RTSPURL, Private) 
						values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
						on conflict(ID) do update set
							Name=excluded.Name, AspectRatio=excluded.AspectRatio, Address=excluded.Address, Diurnal=excluded.Diurnal, Dewarp=excluded.Dewarp, 
							Latitude=excluded.Latitude, Longitude=excluded.Longitude, Timelapse=excluded.Timelapse, ImageURL=excluded.ImageURL, RTSPURL=excluded.RTSPURL, Private=excluded.Private`
	diurnal := 0
	if c.Diurnal {
		diurnal = 1
	}
	dewarp := 0
	if c.Dewarp {
		dewarp = 1
	}
	private := 0
	if c.Private {
		private = 1
	}
	if _, err := cxn.Exec(q, c.ID, c.Name, c.Address, diurnal, dewarp, c.Timelapse, c.StillURL, c.RTSPURL, private); err != nil {
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

// LocalDaylight returns current time, sunrise, and sunset for current moment, in the camera's local time.
func (c *Camera) LocalDaylight(on time.Time) (now time.Time, rise time.Time, set time.Time) {
	// determine our timezone from lat/long
	loc := c.Location()
	if loc == nil {
		return
	}

	if on.IsZero() {
		now = time.Now().In(loc)
	} else {
		now = on
	}

	rise, set = sunrise.SunriseSunset(c.Latitude, c.Longitude, now.Year(), now.Month(), now.Day())

	rise = rise.Add(-35 * time.Minute).In(loc)
	set = set.Add(45 * time.Minute).In(loc)

	return
}

// Location returns a time.Location for the camera, according to its Latitude & Longitude.
func (c *Camera) Location() *time.Location {
	if tz := latlong.LookupZoneName(c.Latitude, c.Longitude); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return nil
}

// IsDark indicates whether the camera is currently offline/sleeping due to
// darkness. If the camera is not diurnal, this always returns false.
func (c *Camera) IsDark() bool {
	// if we're not diurnal, or if location is apparently nonsense, we're never dark
	if !c.Diurnal || (c.Latitude == 0.0 && c.Longitude == 0.0) {
		return false
	}

	now, rise, set := c.LocalDaylight(time.Time{})

	return now.Before(rise) || now.After(set)
}

// User represents an email (specifically, Google/Gmail) account that is permitted to access this
// system via OAuth2. It also records a meatspace name for that user.
type User struct {
	Email      string
	Name       string
	Privileged bool
}

// Store records a new User to the database, or updates it if it already exists.
func (u *User) Store() {
	cxn := System.getDB()
	defer cxn.Close()

	if _, err := cxn.Exec("insert into Users (Email, Name, Privileged) values (?, ?, ?) on conflict(email) do update set Name=excluded.Name, Privileged=excluded.Privileged", u.Email, u.Name); err != nil {
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

// Users returns a list of all User rows currently configured. If there are no users, returns a nil
// slice.
func (sys *SystemConfig) Users() []*User {
	cxn := sys.getDB()
	defer cxn.Close()

	if rows, err := cxn.Query("select Email, Name, Privileged from Users"); err != nil {
		panic(err)
	} else {
		defer rows.Close()

		ret := []*User{}
		for rows.Next() {
			u := &User{}
			rows.Scan(&u.Email, &u.Name, &u.Privileged)
			if u.Email == "" || u.Name == "" {
				panic(fmt.Errorf("user entry loaded with null fields '%s'/'%s'", u.Email, u.Name))
			}
			ret = append(ret, u)
		}

		sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
		return ret
	}
}

// SystemConfig abstracts the configuration database and also provides a central point for accessing
// various runtime settings.
type SystemConfig struct {
	HomeURL         string
	ServiceName     string
	SessionCookieID string
	CameraIDHeader  string
	PollInterval    int
	SqlitePath      string
	DefaultImage    string
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
				"DefaultImage":    &sys.DefaultImage,
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

	if rows, err := cxn.Query("select Name, ID, AspectRatio, Address, Diurnal, Dewarp, Latitude, Longitude, Timelapse, ImageURL, RTSPURL, Private from Cameras"); err != nil {
		panic(err)
	} else {
		defer rows.Close()

		ret := []*Camera{}
		for rows.Next() {
			c := &Camera{}
			rows.Scan(&c.Name, &c.ID, &c.AspectRatio, &c.Address, &c.Diurnal, &c.Dewarp, &c.Latitude, &c.Longitude, &c.Timelapse, &c.StillURL, &c.RTSPURL, &c.Private)
			if c.Name == "" || c.ID == "" {
				panic(fmt.Errorf("camera entry stored with null fields '%s'/'%s'", c.ID, c.Name))
			}
			ret = append(ret, c)
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

	row := cxn.QueryRow("select Name, ID, AspectRatio, Address, Diurnal, Dewarp, Latitude, Longitude, Timelapse, ImageURL, RTSPURL, Private from Cameras where ID=?", ID)

	c := &Camera{}
	err := row.Scan(&c.Name, &c.ID, &c.AspectRatio, &c.Address, &c.Diurnal, &c.Dewarp, &c.Latitude, &c.Longitude, &c.Timelapse, &c.StillURL, &c.RTSPURL, &c.Private)
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

	row := cxn.QueryRow("select Email, Name, Privileged from Users where Email=?", email)

	u := &User{}
	err := row.Scan(&u.Email, &u.Name, &u.Privileged)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		panic(err)
	}
	return u
}
