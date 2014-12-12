package config

type ConfigReader interface {
	ReadConfig() *Config
}
