/*
do-dyndns is a simple dynamic DNS client for DigitalOcean.
It updates one or more DNS records with the current public IP address.
It is intended to be run as a cron job or a systemd service.
*/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/digitalocean/godo"
	"golang.org/x/sys/unix"
)

const Prog = "do-dyndns"
const Version = "1.2.0"

// ConfigFile names.
const ConfigFile = "config.json"
const DotConfigFile = "." + Prog + ".json"

const Usage = `Usage: %s [OPTIONS]

OPTIONS
    -h, --help                display this help and exit
    -v, --version             display version information and exit
    --token string            DigitalOcean API token (overrides DYNDNS_TOKEN)
    --log string              log file path (overrides DYNDNS_LOG)
    --type string             DNS record type (A or AAAA) (default "A")
    --subdomain string        Subdomain to update (e.g. "www.example.com")

FILES
    $HOME/.config/%s/config.json

ENVIRONMENT
    DYNDNS_TOKEN        DigitalOcean API token
    DYNDNS_LOG          log file path
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
		slog.Info(text)
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
		slog.Warn(text)
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

// Create a HTTP client for IPv4 connections only.
func createIPv4Client() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
		},
	}

	return &http.Client{
		Transport: transport,
	}
}

// myPublicIP returns the public IPv4 address of the machine.
func myPublicIP() (ip net.IP, err error) {
	client := createIPv4Client()
	resp, err := client.Get("https://ifconfig.co/ip")
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Trim any whitespace from the response
	ipStr := strings.TrimSpace(string(body))

	ip = net.ParseIP(ipStr)
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

func parseArguments() (bool, bool, string, string, string, string) {
	var help, version bool
	var token, logFile, recordType, subdomain string

	flag.BoolVar(&help, "h", false, "display help")
	flag.BoolVar(&help, "help", false, "display help")
	flag.BoolVar(&version, "v", false, "display version")
	flag.BoolVar(&version, "version", false, "display version")
	flag.StringVar(&token, "token", "", "DigitalOcean API token (overrides DYNDNS_TOKEN)")
	flag.StringVar(&logFile, "log", "", "log file path (overrides DYNDNS_LOG)")
	flag.StringVar(&recordType, "type", "A", "DNS record type (A or AAAA)")
	flag.StringVar(&subdomain, "subdomain", "", "Subdomain to update")
	flag.Parse()

	return help, version, token, logFile, recordType, subdomain
}

// RUN.
func main() {
	help, version, token, logFile, recordType, subdomain := parseArguments()
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

	config, err := readConfig(token, logFile)
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

	if subdomain != "" {
		// Use ad-hoc record if subdomain was provided via command line
		adHocRecords := []Record{
			{
				Type:      recordType,
				Subdomain: subdomain,
			},
		}
		setSubdomainRecords(config.Token, &adHocRecords, ip)
	} else {
		// Use records from config file
		setSubdomainRecords(config.Token, &config.Records, ip)
	}
}
