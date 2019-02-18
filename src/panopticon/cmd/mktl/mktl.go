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

package main

import (
	"panopticon"
	"time"

	"playground/config"
	"playground/log"
	"playground/session"
)

type serverConfig struct {
	Hostname        string
	BindAddress     string
	Port            int
	HTTPPort        int
	TLSCertPath     string
	TLSKeyPath      string
	StaticPath      string
	PreloadList     []string
	CameraAPISecret *struct{ Header, Value string }
}

var cfg = &struct {
	Debug      bool
	LogFile    string
	Server     *serverConfig
	System     *panopticon.SystemConfig
	Repository *panopticon.RepositoryConfig
	Session    *session.ConfigType
}{
	true,
	"",
	&serverConfig{
		"", "", 443, 80, "./server.crt", "./server.key", "./static",
		[]string{
			"index.html", "panopticon.css", "panopticon.js", "favicon.ico",
			"no-image.png", "manifest.json", "icon-192.png", "icon-512.png",
		},
		&struct{ Header, Value string }{},
	},
	panopticon.System,
	panopticon.Repository,
	&session.Config,
}

func initConfig() {
	config.Load(cfg)
	if cfg.LogFile != "" {
		log.SetLogFile(cfg.LogFile)
	}
	if cfg.Debug || config.Debug {
		log.SetLogLevel(log.LEVEL_DEBUG)
	}
	cfg.System.Ready()
	cfg.Repository.Ready()
}

func main() {
	initConfig()
	session.Ready()

	today := time.Now().Local()

	cam := panopticon.System.GetCamera("dachacam")
	for i := 1; i < 3; i++ {
		day := today.Add(time.Duration(-i) * 24 * time.Hour)
		log.Debug("wtf", "??", day)
		panopticon.Repository.GenerateTimelapse(day, cam, panopticon.MediaCollected)
	}

	//panopticon.Repository.PurgeBefore(panopticon.MediaCollected, time.Date(today.Year(), today.Month(), today.Day()-1, 0, 0, 0, 0, time.Local))

	for {
		time.Sleep(1000000)
	}
}
