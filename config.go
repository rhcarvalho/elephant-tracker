package main

import (
	"encoding/json"
	"io"
	"os"
	_path "path"
)

type Config struct {
	Http  *HttpConfig  `json:"http"`
	Mongo *MongoConfig `json:"mongo"`
}

type HttpConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type MongoConfig struct {
	URL string `json:"url"`
	DB  string `json:"db"`
}

// ConfigOpen opens a configuration file and returns a Config.
func ConfigOpen(path string) (*Config, error) {
	path, err := absPath(os.ExpandEnv(path))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf, err := configNew(f)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// configNew decodes JSON from a io.Reader into a Config.
func configNew(raw io.Reader) (*Config, error) {
	conf := &Config{}
	err := json.NewDecoder(raw).Decode(conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// absPath translates relative paths into absolute paths.
func absPath(path string) (string, error) {
	if _path.IsAbs(path) {
		return path, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return path, err
	}
	return _path.Join(wd, path), nil
}
