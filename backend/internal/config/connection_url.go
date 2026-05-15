package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type ParsedDatabaseURL struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type ParsedRedisURL struct {
	Host      string
	Port      int
	Password  string
	DB        int
	EnableTLS bool
}

func ParseDatabaseURL(raw string) (*ParsedDatabaseURL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("empty database url")
	}

	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if scheme := strings.ToLower(strings.TrimSpace(u.Scheme)); scheme != "postgres" && scheme != "postgresql" {
		return nil, fmt.Errorf("unsupported database url scheme: %s", u.Scheme)
	}
	if strings.TrimSpace(u.Hostname()) == "" {
		return nil, fmt.Errorf("database url missing host")
	}

	port := 5432
	if rawPort := strings.TrimSpace(u.Port()); rawPort != "" {
		parsedPort, parseErr := strconv.Atoi(rawPort)
		if parseErr != nil || parsedPort <= 0 {
			return nil, fmt.Errorf("database url has invalid port: %s", rawPort)
		}
		port = parsedPort
	}

	password, _ := u.User.Password()
	dbName := strings.TrimPrefix(strings.TrimSpace(u.Path), "/")

	return &ParsedDatabaseURL{
		Host:     strings.TrimSpace(u.Hostname()),
		Port:     port,
		User:     strings.TrimSpace(u.User.Username()),
		Password: password,
		DBName:   dbName,
		SSLMode:  firstNonEmptyString(strings.TrimSpace(u.Query().Get("sslmode")), "prefer"),
	}, nil
}

func ParseRedisURL(raw string) (*ParsedRedisURL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("empty redis url")
	}

	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	switch scheme {
	case "redis", "rediss":
	default:
		return nil, fmt.Errorf("unsupported redis url scheme: %s", u.Scheme)
	}

	if strings.TrimSpace(u.Hostname()) == "" {
		return nil, fmt.Errorf("redis url missing host")
	}

	port := 6379
	if rawPort := strings.TrimSpace(u.Port()); rawPort != "" {
		parsedPort, parseErr := strconv.Atoi(rawPort)
		if parseErr != nil || parsedPort <= 0 {
			return nil, fmt.Errorf("redis url has invalid port: %s", rawPort)
		}
		port = parsedPort
	}

	password, _ := u.User.Password()
	db := 0
	if rawDB := strings.TrimPrefix(strings.TrimSpace(u.Path), "/"); rawDB != "" {
		parsedDB, parseErr := strconv.Atoi(rawDB)
		if parseErr != nil || parsedDB < 0 {
			return nil, fmt.Errorf("redis url has invalid database: %s", rawDB)
		}
		db = parsedDB
	} else if queryDB := strings.TrimSpace(u.Query().Get("db")); queryDB != "" {
		parsedDB, parseErr := strconv.Atoi(queryDB)
		if parseErr != nil || parsedDB < 0 {
			return nil, fmt.Errorf("redis url has invalid database: %s", queryDB)
		}
		db = parsedDB
	}

	enableTLS := scheme == "rediss"
	for _, key := range []string{"ssl", "tls"} {
		if rawFlag := strings.TrimSpace(u.Query().Get(key)); rawFlag != "" {
			parsedFlag, parseErr := strconv.ParseBool(rawFlag)
			if parseErr != nil {
				return nil, fmt.Errorf("redis url has invalid %s flag: %s", key, rawFlag)
			}
			enableTLS = parsedFlag
		}
	}

	return &ParsedRedisURL{
		Host:      strings.TrimSpace(u.Hostname()),
		Port:      port,
		Password:  password,
		DB:        db,
		EnableTLS: enableTLS,
	}, nil
}
