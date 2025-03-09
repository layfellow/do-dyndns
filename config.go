/*
do-dyndns is a simple dynamic DNS client for DigitalOcean.
It updates one or more DNS records with the current public IP address.
It is intended to be run as a cron job or a systemd service.
*/
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// readConfig reads the configuration from files and environment variables.
func readConfig(cmdToken, cmdLog string) (config Config, err error) {
	// Try to read from config file first
	config, err = readConfigFile()
	if err != nil {
		return config, err
	}

	// Environment variables override config file
	envToken := os.Getenv("DYNDNS_TOKEN")
	if envToken != "" {
		config.Token = envToken
	}

	envLog := os.Getenv("DYNDNS_LOG")
	if envLog != "" {
		config.Log = envLog
	}

	// Command line arguments override environment variables and config file
	if cmdToken != "" {
		config.Token = cmdToken
	}

	if cmdLog != "" {
		config.Log = cmdLog
	}

	return config, nil
}

// readConfigFile attempts to read the configuration from a file.
func readConfigFile() (config Config, err error) {
	// Try user config directory first
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return config, err
	}

	configDir := filepath.Join(userConfigDir, Prog)
	configPath := filepath.Join(configDir, ConfigFile)

	// Create the config directory if it doesn't exist
	if _, err = os.Stat(configDir); err != nil {
		if err = os.MkdirAll(configDir, 0755); err != nil {
			return config, err
		}
	}

	// Try to read the config file
	data, err := os.ReadFile(configPath)

	if err != nil {
		// If not found in primary location, try legacy location
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return config, err
		}

		data, err = os.ReadFile(filepath.Join(userHomeDir, DotConfigFile))
		if err != nil {
			return config, errors.New("unable to find config file")
		}
	}

	// Parse the JSON data
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}
