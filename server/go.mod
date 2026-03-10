module github.com/linkterm/linkterm/server

go 1.21

replace github.com/linkterm/linkterm/proto => ../proto

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/linkterm/linkterm/proto v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
	nhooyr.io/websocket v1.8.17
)
