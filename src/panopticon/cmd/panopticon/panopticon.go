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
	"fmt"

	"panopticon"

	"playground/config"
	"playground/httputil"
	"playground/httputil/static"
	"playground/log"
	"playground/session"
)

type serverConfig struct {
	Hostname        string
	BindAddress     string
	Port            int
	HTTPPort        int
	TLSKeypairs     [][]string
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
		"", "", 443, 80, [][]string{[]string{"./server.crt", "./server.key"}}, "./static",
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

func emailInspector(email string) bool {
	user := panopticon.System.GetUser(email)
	log.Debug("emailInspector", fmt.Sprintf("'%s' is authorized: %t", email, user != nil))
	return user != nil
}

func main() {
	initConfig()
	session.Ready()

	server, mux := httputil.NewHardenedServer(cfg.Server.BindAddress, cfg.Server.Port)

	w := httputil.Wrapper().WithPanicHandler().WithMethodSentry("GET")

	// Single API endpoint for provisioning QR
	mux.HandleFunc("/client/provision", w.Wrap(panopticon.ProvisionHandler))

	// OAuth2 session login handler
	mux.HandleFunc(session.Config.OAuth.RedirectPath, w.Wrap(static.OAuthHandler))

	// web UI static assets
	content := static.Content{Path: cfg.Server.StaticPath, Prefix: "/static/", DisablePreloading: cfg.Debug}
	content.Preload(cfg.Server.PreloadList...)
	mux.HandleFunc("/", content.RootHandler) // must be unprotected as it starts the login flow
	w = httputil.Wrapper().WithPanicHandler().WithSessionSentry(panopticon.AuthError).WithAuthCallback(panopticon.AuthError, emailInspector)
	mux.HandleFunc("/favicon.ico", w.WithMethodSentry("GET").Wrap(content.FaviconHandler))
	mux.HandleFunc("/static/", w.WithMethodSentry("GET").Wrap(content.Handler))

	// API endpoints for client UI
	mux.HandleFunc("/client/state", w.WithMethodSentry("GET").Wrap(panopticon.StateHandler))
	mux.HandleFunc("/client/image/", w.WithMethodSentry("GET").Wrap(panopticon.ImageHandler))
	mux.HandleFunc("/client/video/", w.WithMethodSentry("GET").Wrap(panopticon.ImageHandler))
	mux.HandleFunc("/client/imagemeta/", w.WithMethodSentry("GET").Wrap(panopticon.ImageMetaHandler))
	mux.HandleFunc("/client/images/", w.WithMethodSentry("GET").Wrap(panopticon.ImageListHandler))
	mux.HandleFunc("/client/save/", w.WithMethodSentry("PUT").Wrap(panopticon.SaveHandler))

	// API endpoints for camera clients
	w = httputil.Wrapper().WithPanicHandler().WithSecretSentry(cfg.Server.CameraAPISecret.Header, cfg.Server.CameraAPISecret.Value)
	mux.HandleFunc("/camera/motion", w.WithMethodSentry("POST").Wrap(panopticon.MotionHandler))
	mux.HandleFunc("/camera/latest", w.WithMethodSentry("POST").Wrap(panopticon.LatestHandler))

	// start up an HSTS redirector to our TLS port
	httputil.Config.EnableHSTS = true
	server.ListenAndServeTLSRedirector(cfg.Server.Hostname, cfg.Server.HTTPPort)

	// start up the HTTPS server
	log.Error("main", "shutting down", server.ListenAndServeSNI(cfg.Server.TLSKeypairs))
}
