module terminal-example

go 1.25.6

require (
	github.com/creack/pty v1.1.24
	github.com/gorilla/websocket v1.5.3
	godom v0.0.0
)

require (
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	golang.org/x/net v0.52.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace godom => ../..
