module github.com/linkterm/linkterm/agent

go 1.24.0

toolchain go1.24.3

replace github.com/linkterm/linkterm/proto => ../proto

require (
	fyne.io/systray v1.12.0
	github.com/creack/pty v1.1.24
	github.com/linkterm/linkterm/proto v0.0.0-00010101000000-000000000000
	golang.org/x/term v0.40.0
	gopkg.in/yaml.v3 v3.0.1
	nhooyr.io/websocket v1.8.17
)

require (
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
