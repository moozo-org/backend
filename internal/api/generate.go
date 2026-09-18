package api

//go:generate npx --no-install redocly bundle server.yaml -o bundled/server.yaml
//go:generate go tool ogen --target generated -package generated --clean bundled/server.yaml
