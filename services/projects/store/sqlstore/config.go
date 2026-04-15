package sqlstore

import (
	"net/url"
	"strings"
)

type Config struct {
	PostgresUser     string `env:"POSTGRES_USER" env-default:"postgres" env-description:"Postgres connection user"`
	PostgresPassword string `env:"POSTGRES_PASSWORD" env-default:"postgres" env-description:"Postgres connection password"`
	PostgresHost     string `env:"POSTGRES_HOST" env-default:"localhost" env-description:"Postgres connection host"`
	PostgresPort     string `env:"POSTGRES_PORT" env-default:"5432" env-description:"Postgres connection port"`
	PostgresDB       string `env:"POSTGRES_DB" env-default:"postgres" env-description:"Postgres connection database"`
	PostgresSSLMode  string `env:"POSTGRES_SSLMODE" env-default:"require" env-description:"Postgres connection sslmode"`
}

func (c *Config) ConnectionString() string {
	encodedUser := url.QueryEscape(c.PostgresUser)
	encodedPassword := url.QueryEscape(c.PostgresPassword)
	encodedDB := url.QueryEscape(c.PostgresDB)

	return "postgres://" + encodedUser + ":" + encodedPassword + "@" + c.PostgresHost + ":" + c.PostgresPort + "/" + encodedDB + "?sslmode=" + c.PostgresSSLMode
}

func ConfigFromConnectionString(connStr string) (Config, error) {
	url, err := url.Parse(connStr)
	if err != nil {
		return Config{}, err
	}
	pwd, _ := url.User.Password()
	return Config{
		PostgresUser:     url.User.Username(),
		PostgresPassword: pwd,
		PostgresHost:     url.Hostname(),
		PostgresPort:     url.Port(),
		PostgresDB:       strings.TrimPrefix(url.Path, "/"),
		PostgresSSLMode:  url.Query().Get("sslmode"),
	}, nil
}
