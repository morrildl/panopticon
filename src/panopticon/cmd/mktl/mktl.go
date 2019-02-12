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
