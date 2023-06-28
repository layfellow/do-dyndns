/*
do-dyndns is a simple dynamic DNS client for DigitalOcean.
It updates a DNS record with the current public IP address.
It is intended to be run as a cron job or a systemd service.

Usage:

	do-dyndns
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/jbrodriguez/mlog"
	"golang.org/x/sys/unix"
)

const PROG = "do-dyndns"

// Configuration file names.
const CONFIG = "config.json"
const DOT_CONFIG = "." + PROG + ".json"

// Log file name and parameters passed to mlog.
const LOGFILE = "out.log"
const LOGFILE_COUNT = 3
const LOGFILE_SIZE = 128 * 1024

const USAGE = `Usage: %s [OPTIONS]

OPTIONS
    -h, --help    display this help and exit

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
	tty     bool = isatty()
	systemd bool = isSystemdService()
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
		fmt.Fprintln(os.Stdout, text)
	} else {
		mlog.Info(text)
	}
}

// writeErr writes to stderr or the log file, depending on the environment.
func writeErr(text string) {
	if tty || systemd {
		fmt.Fprintln(os.Stderr, text)
	} else {
		mlog.Warning(text)
	}
}

// die writes an error message to stderr or the log file and then exits.
func die(text string, err error) {
	if err != nil {
		writeErr(fmt.Sprintf("%s: %s; %s", PROG, text, err))
	} else {
		writeErr(fmt.Sprintf("%s: %s", PROG, text))
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
		logDir = filepath.Join(userCacheDir, PROG)
		logfile = filepath.Join(logDir, LOGFILE)
	}

	// Create the log directory if it doesn't exist.
	if _, err = os.Stat(logDir); err != nil {
		if err = os.MkdirAll(logDir, 0755); err != nil {
			return
		}
	}
	mlog.StartEx(mlog.LevelInfo, logfile, LOGFILE_SIZE, LOGFILE_COUNT)
	return nil
}

// readConfig reads the configuration file.
func readConfig() (config Config, err error) {
	var userHomeDir string
	userHomeDir, err = os.UserHomeDir()
	if err != nil {
		return
	}

	// userConfigDir is $HOME/.config on Linux.
	var userConfigDir string
	userConfigDir, err = os.UserConfigDir()
	if err != nil {
		return
	}

	// Create the config directory if it doesn't exist.
	configDir := filepath.Join(userConfigDir, PROG)
	if _, err = os.Stat(configDir); err != nil {
		if err = os.MkdirAll(configDir, 0755); err != nil {
			return
		}
	}

	// Look for the config file in the config directory.
	configFile := filepath.Join(configDir, CONFIG)
	if _, err = os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		// If it doesn't exist, look for the old style config file in $HOME.
		configFile = filepath.Join(userHomeDir, DOT_CONFIG)
		if _, err = os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
			return config, errors.New("unable to find config file")
		}
	}

	var content []byte
	content, err = os.ReadFile(configFile)
	if err != nil {
		return
	}

	// Substitute $HOME with the actual home directory
	content = []byte(os.ExpandEnv(string(content)))

	// Parse the JSON data in config file.
	err = json.Unmarshal(content, &config)
	if err != nil {
		return
	}
	return config, nil
}

// myPublicIP returns the public IPv4 address of the machine.
func myPublicIP() (ip net.IP, err error) {
	// Use ip-api.com to get the public IP address.
	resp, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var v map[string]interface{}
	err = decoder.Decode(&v)
	if err != nil {
		return
	}
	ip = net.ParseIP(v["query"].(string))
	if ip == nil {
		err = errors.New("no IPv4 found")
	}
	return ip, nil
}

// setSubdomainIP sets the IP address of a subdomain.
func setSubdomainIP(client *godo.Client, token string, recordType string, subdomain string, ip net.IP) (*godo.Response, error) {
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

		resp, err = setSubdomainIP(client, token, record.Type, record.Subdomain, ip)
		if err != nil {
			die("error setting subdomain IP", err)
		}
		if resp != nil {
			writeOut(fmt.Sprintf("%s: set %s %s for %s", resp.Status, record.Type, ip.String(), record.Subdomain))
		}

	}
}

func parseArguments() bool {
	var help bool
	flag.BoolVar(&help, "h", false, "")
	flag.BoolVar(&help, "help", false, "")
	flag.Parse()
	return help
}

// RUN
func main() {
	help := parseArguments()
	if help {
		fmt.Fprintf(os.Stderr, USAGE, PROG, PROG)
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
