package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge/addon"
)

type config struct {
	// Woodpecker
	repoDir string
	url     string

	// OIDC
	clientID         string
	clientSecret     string
	clientSecretFile string
	clientProvider   string
	clientRedirect   string

	// Misc
	logFilePath string
	logJSON     bool
}

func main() {
	cfg := &config{
		repoDir:          mustEnv("repos"),
		url:              mustEnv("url"),
		clientID:         mustEnv("client_id"),
		clientSecret:     env("client_secret"),
		clientSecretFile: env("client_secret_file"),
		clientProvider:   mustEnv("provider"),
		clientRedirect:   mustEnv("redirect"),
		logFilePath:      env("log_file"),
		logJSON:          env("log_json") == "1",
	}

	if cfg.clientSecretFile != "" {
		content, err := os.ReadFile(cfg.clientSecretFile)
		if err != nil {
			panic(err)
		}
		cfg.clientSecret = strings.TrimSpace(string(content))
	}
	if cfg.clientSecret == "" {
		panic("client secret is required")
	}

	logLevel := slog.LevelInfo
	switch env("log_level") {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logOutput := os.Stderr
	if cfg.logFilePath != "" {
		fi, err := os.Create(cfg.logFilePath)
		if err != nil {
			panic(err)
		}
		defer fi.Close()
		logOutput = fi
	}

	logOpts := &slog.HandlerOptions{Level: logLevel}
	var logger slog.Handler
	if cfg.logJSON {
		logger = slog.NewJSONHandler(logOutput, logOpts)
	} else {
		logger = slog.NewTextHandler(logOutput, logOpts)
	}
	slog.SetDefault(slog.New(logger))

	defer func() {
		if r := recover(); r != nil {
			slog.Info("addon forge panic", slog.Any("message", r))
		}
	}()
	addon.Serve(cfg)
	slog.Debug("bye bye")
}

func env(stem string) string {
	name := fmt.Sprintf("GITPECKER_%s", strings.ToUpper(stem))
	return os.Getenv(name)
}

func mustEnv(stem string) string {
	name := fmt.Sprintf("GITPECKER_%s", strings.ToUpper(stem))
	val, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("%q is a required environment variable", name))
	}
	return val
}
