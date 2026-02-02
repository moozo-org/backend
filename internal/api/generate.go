package api

//go:generate redocly bundle server.yaml -o bundled/server.yaml --config config.yaml
//go:generate redocly bundle server.yaml -o bundled/swagger.yaml
//go:generate go run github.com/ogen-go/ogen/cmd/ogen --target generated -package generated --clean bundled/server.yaml
