/*
do-dyndns is a simple dynamic DNS client for DigitalOcean.
It updates one or more DNS records with the current public IP address.
It is intended to be run as a cron job or a systemd service.
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/jbrodriguez/mlog"
	"golang.org/x/sys/unix"
)

const Prog = "do-dyndns"
const Version = "1.0.1"

// ConfigFile names.
const ConfigFile = "config.json"
const DotConfigFile = "." + Prog + ".json"

// LogFile name and parameters passed to mlog.
const LogFile = "out.log"
const LogFileCount = 3
const LogFileSize = 128 * 1024

const Usage = `Usage: %s [OPTIONS]

OPTIONS
    -h, --help    display this help and exit
    -v, --version display version information and exit

FILES
    $HOME/.config/%s/config.json
`

type Record struct {
	Type      string `json:"type"`
	Subdomain string `json:"subdomain"`
}

// Config is the configuration file format.
type Config struct {
	Log     string   `json:"log"`
	Token   string   `json:"token"`
	Records []Record `json:"records"`
}

// Global variables describing the environment do-dyndns is running in.
var (
	tty     = isatty()
	systemd = isSystemdService()
)

// isatty returns true if stdout is a terminal.
func isatty() bool {
	_, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)

	return err == nil
}

// isSystemdService returns true if do-dyndns is running as a systemd service.
func isSystemdService() bool {
	_, ok := os.LookupEnv("SYSTEMD_EXEC_PID")

	return ok
}

// writeOut writes to stdout or the log file, depending on the environment.
func writeOut(text string) {
	if tty || systemd {
		_, err := fmt.Fprintln(os.Stdout, text)
		if err != nil {
			return
		}
	} else {
		mlog.Info(text)
	}
}

// writeErr writes to stderr or the log file, depending on the environment.
func writeErr(text string) {
	if tty || systemd {
		_, err := fmt.Fprintln(os.Stderr, text)
		if err != nil {
			return
		}
	} else {
		mlog.Warning(text)
	}
}

// die writes an error message to stderr or the log file and then exits.
func die(text string, err error) {
	if err != nil {
		writeErr(fmt.Sprintf("%s: %s; %s", Prog, text, err))
	} else {
		writeErr(fmt.Sprintf("%s: %s", Prog, text))
	}

	os.Exit(1)
}

// initLogger initializes mlog.
func initLogger(logfile string) (err error) {
	var logDir string

	// If logfile is explicitly set, use it.
	if logfile != "" {
		logDir = filepath.Dir(logfile)
	} else {
		// Otherwise, use the user cache directory.
		// On Linux, this is $HOME/.cache.
		var userCacheDir string
		userCacheDir, err = os.UserCacheDir()
		if err != nil {
			return
		}
		logDir = filepath.Join(userCacheDir, Prog)
		logfile = filepath.Join(logDir, LogFile)
	}

	// Create the log directory if it doesn't exist.
	if _, err = os.Stat(logDir); err != nil {
		if err = os.MkdirAll(logDir, 0755); err != nil {
			return
		}
	}

	mlog.StartEx(mlog.LevelInfo, logfile, LogFileSize, LogFileCount)

	return nil
}

// readConfig reads the configuration file.
func readConfig() (config Config, err error) {
	var userHomeDir string

	userHomeDir, err = os.UserHomeDir()
	if err != nil {
		return config, err
	}

	// userConfigDir is $HOME/.config on Linux.
	var userConfigDir string

	userConfigDir, err = os.UserConfigDir()
	if err != nil {
		return config, err
	}

	// Create the config directory if it doesn't exist.
	configDir := filepath.Join(userConfigDir, Prog)
	if _, err = os.Stat(configDir); err != nil {
		if err = os.MkdirAll(configDir, 0755); err != nil {
			return config, err
		}
	}

	// Look for the config file in the config directory.
	configFile := filepath.Join(configDir, ConfigFile)
	if _, err = os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		// If it doesn't exist, look for the old style config file in $HOME.
		configFile = filepath.Join(userHomeDir, DotConfigFile)
		if _, err = os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
			return config, errors.New("unable to find config file")
		}
	}

	var content []byte

	content, err = os.ReadFile(configFile)
	if err != nil {
		return config, err
	}

	// Substitute $HOME with the actual home directory
	content = []byte(os.ExpandEnv(string(content)))

	// Parse the JSON data in config file.
	err = json.Unmarshal(content, &config)
	if err != nil {
		return config, err
	}

	return config, err
}

// myPublicIP returns the public IPv4 address of the machine.
func myPublicIP() (ip net.IP, err error) {
	resp, err := http.Get("https://api4.ipify.org")

	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)

	ip = net.ParseIP(string(body))
	if ip == nil {
		err = errors.New("no IPv4 found")
	}

	return ip, err
}

// setSubdomainIP sets the IP address of a subdomain.
func setSubdomainIP(client *godo.Client, recordType string, subdomain string, ip net.IP) (*godo.Response, error) {
	i := strings.Index(subdomain, ".")
	if i < 0 {
		die(fmt.Sprintf("invalid subdomain, %s", subdomain), nil)
	}

	name := subdomain[:i]
	domain := subdomain[i+1:]

	ctx := context.TODO()

	// Get the existing DNS records to avoid creating duplicates.
	records, _, err := client.Domains.Records(ctx, domain, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	var resp *godo.Response

	for _, record := range records {
		if record.Type == recordType && record.Name == name {
			if record.Data != ip.String() {
				// Update an existing DNS record.
				_, resp, err = client.Domains.EditRecord(ctx, domain, record.ID, &godo.DomainRecordEditRequest{
					Type: recordType,
					Name: name,
					Data: ip.String(),
				})

				return resp, err
			} else {
				// Do nothing if the IP address is the same.
				return nil, nil
			}
		}
	}

	// Create a new DNS record.
	_, resp, err = client.Domains.CreateRecord(ctx, domain, &godo.DomainRecordEditRequest{
		Type: recordType,
		Name: name,
		Data: ip.String(),
	})

	return resp, err
}

// setSubdomainRecords sets the IP address of multiple subdomains.
func setSubdomainRecords(token string, records *[]Record, ip net.IP) {
	client := godo.NewFromToken(token)

	var resp *godo.Response

	var err error

	for _, record := range *records {
		if record.Type != "A" && record.Type != "AAAA" {
			die(fmt.Sprintf("invalid type, %s", record.Type), nil)
		}

		if record.Subdomain == "" {
			die("missing subdomain", nil)
		}

		resp, err = setSubdomainIP(client, record.Type, record.Subdomain, ip)
		if err != nil {
			die("error setting subdomain IP", err)
		}

		if resp != nil {
			writeOut(fmt.Sprintf("%s: set %s %s for %s", resp.Status, record.Type, ip.String(), record.Subdomain))
		}
	}
}

func parseArguments() (bool, bool) {
	var help, version bool

	flag.BoolVar(&help, "h", false, "")
	flag.BoolVar(&help, "help", false, "")
	flag.BoolVar(&version, "v", false, "")
	flag.BoolVar(&version, "version", false, "")
	flag.Parse()

	return help, version
}

// RUN.
func main() {
	help, version := parseArguments()
	if help {
		_, err := fmt.Fprintf(os.Stderr, Usage, Prog, Prog)
		if err != nil {
			os.Exit(1)
		}

		os.Exit(0)
	} else if version {
		_, err := fmt.Fprintf(os.Stderr, "%s %s\n", Prog, Version)
		if err != nil {
			os.Exit(1)
		}

		os.Exit(0)
	}

	config, err := readConfig()
	if err != nil {
		die("error reading configuration", err)
	}

	if !tty && !systemd {
		err := initLogger(config.Log)
		if err != nil {
			die("error writing to log file", err)
		}
	}

	if config.Token == "" {
		die("missing token", nil)
	}

	var ip net.IP

	ip, err = myPublicIP()
	if err != nil {
		die("error getting public IP", err)
	}

	setSubdomainRecords(config.Token, &config.Records, ip)
}
