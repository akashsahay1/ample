package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is the desktop app version shown in the UI (the lead wires it to core.Version).
var version = "1.0.0"

// build is the build number (git commit count), set via -ldflags "-X main.build=N".
var build = ""

// startHidden reports whether --hidden was passed (used by launch-at-login).
func startHidden() bool {
	for _, a := range os.Args[1:] {
		if a == "--hidden" || a == "-hidden" {
			return true
		}
	}
	return false
}

func main() {
	app := NewApp(newBackend())

	startTray(app)

	err := wails.Run(&options.App{
		Title:            "AMPLS",
		Width:            1280,
		Height:           800,
		MinWidth:         1024,
		MinHeight:        680,
		Frameless:        true,
		StartHidden:      trayAvailable && startHidden(),
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 0xF5, G: 0xF3, B: 0xEF, A: 255},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.ampls.app",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.ShowWindow()
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			Theme:                             windows.Light,
			DisableFramelessWindowDecorations: false,
		},
		Bind: []interface{}{app},
	})
	if err != nil {
		log.Println("Error:", err.Error())
	}
}
