package api

//go:generate redocly bundle server.yaml -o bundled/server.yaml --dereferenced
//go:generate go run github.com/ogen-go/ogen/cmd/ogen --target generated -package generated --clean bundled/server.yaml
