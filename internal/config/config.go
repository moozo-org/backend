package config

import "github.com/kelseyhightower/envconfig"

type Environment struct {
	Production bool `envconfig:"PRODUCTION" default:"false"`
}

func Load[T any]() (T, error) {
	var c T
	err := envconfig.Process("", &c)
	return c, err
}
