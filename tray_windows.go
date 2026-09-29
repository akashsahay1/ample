//go:build windows

package main

import (
	"fmt"
	goruntime "runtime"
	"time"

	"github.com/energye/systray"
)

const trayAvailable = true

func startTray(app *App) {
	go func() {
		goruntime.LockOSThread()
		systray.Run(func() { trayReady(app) }, func() {})
	}()
}

func stopTray() { systray.Quit() }

func trayReady(app *App) {
	icons := map[string][]byte{"running": iconRunning(), "stopped": iconStopped(), "partial": iconPartial()}
	systray.SetIcon(icons["stopped"])
	systray.SetTitle("AMPLS")
	systray.SetTooltip("AMPLS")
	systray.SetOnClick(func(systray.IMenu) { app.ShowWindow() })
	systray.SetOnDClick(func(systray.IMenu) { app.ShowWindow() })

	systray.AddMenuItem("Open AMPLS", "Show the AMPLS window").Click(app.ShowWindow)
	systray.AddSeparator()
	systray.AddMenuItem("Start all", "Start Apache and MySQL").Click(func() { go app.StartAll() })
	systray.AddMenuItem("Stop all", "Stop Apache and MySQL").Click(func() { go app.StopAll() })
	systray.AddMenuItem("Restart all", "Restart Apache and MySQL").Click(func() { go app.RestartAll() })
	systray.AddSeparator()
	systray.AddMenuItem("Open Sites folder", "").Click(func() {
		if s, err := app.b.GetSettings(); err == nil && len(s.Parked) > 0 {
			_ = openFolder(s.Parked[0])
		}
	})
	systray.AddMenuItem("phpMyAdmin", "Open phpMyAdmin in the browser").Click(func() {
		if app.ctx != nil {
			app.OpenURL("http://localhost/phpmyadmin")
		}
	})
	systray.AddSeparator()
	systray.AddMenuItem("Quit AMPLS", "").Click(func() {
		if app.ctx != nil {
			app.Quit()
		}
	})

	last := ""
	update := func() {
		ov, err := app.b.Overview()
		if err != nil {
			return
		}
		stopped := 0
		for _, s := range ov.Services {
			if !s.Running {
				stopped++
			}
		}
		state, tip := "running", "AMPLS — all services running"
		switch {
		case stopped == len(ov.Services):
			state, tip = "stopped", "AMPLS — services stopped"
		case stopped > 0:
			state, tip = "partial", fmt.Sprintf("AMPLS — %d stopped", stopped)
		}
		if state != last {
			systray.SetIcon(icons[state])
			systray.SetTooltip(tip)
			last = state
		}
	}
	update()
	t := time.NewTicker(5 * time.Second)
	for range t.C {
		update()
	}
}
