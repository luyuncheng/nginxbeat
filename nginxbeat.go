package main

import (
	"bufio"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/elastic/libbeat/beat"
	"github.com/elastic/libbeat/cfgfile"
	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/logp"
	"github.com/elastic/libbeat/publisher"
)

// Nginxbeat implements Beater interface and sends Nginx status using libbeat.
type Nginxbeat struct {
	// NbConfig holds configurations of Nginxbeat parsed by libbeat.
	NbConfig ConfigSettings

	isAlive bool
	events  publisher.Client

	url    *url.URL
	period time.Duration
}

// Config Nginxbeat according to nginxbeat.yml.
func (nb *Nginxbeat) Config(b *beat.Beat) error {
	err := cfgfile.Read(&nb.NbConfig, "")
	if err != nil {
		logp.Err("Error reading configuration file: %v", err)
		return err
	}

	var u string
	if nb.NbConfig.Input.URL != "" {
		u = nb.NbConfig.Input.URL
	} else {
		u = "http://127.0.0.1/status"
	}
	nb.url, err = url.Parse(u)
	if err != nil {
		logp.Err("Invalid Nginx status page: %v", err)
		return err
	}

	if nb.NbConfig.Input.Period != nil {
		nb.period = time.Duration(*nb.NbConfig.Input.Period) * time.Second
	} else {
		nb.period = 1 * time.Second
	}

	logp.Debug("nginxbeat", "Init nginxbeat")
	logp.Debug("nginxbeat", "Watch %v\n", nb.url)
	logp.Debug("nginxbeat", "Period %v\n", nb.period)

	return nil
}

// Setup Nginxbeat.
func (nb *Nginxbeat) Setup(b *beat.Beat) error {
	nb.events = b.Events
	return nil
}

// Run Nginxbeat.
func (nb *Nginxbeat) Run(b *beat.Beat) error {
	nb.isAlive = true

	for nb.isAlive {
		nb.exportStubStatus()

		time.Sleep(nb.period)
	}

	return nil
}

// Cleanup Nginxbeat.
func (nb *Nginxbeat) Cleanup(b *beat.Beat) error {
	return nil
}

// Stop Nginxbeat.
func (nb *Nginxbeat) Stop() {
	nb.isAlive = false
}

func (nb Nginxbeat) exportStubStatus() {
	res, err := http.Get(nb.url.String())
	if err != nil {
		logp.Err("Error reading Nginx stub status: %v", err)
		return
	}

	if res.StatusCode != 200 {
		logp.Err("Error reading Nginx stub status: HTTP%v", res.StatusCode)
		return
	}

	// Nginx stub status sample:
	// Active connections: 1
	// server accepts handled requests
	//  7 7 19
	// Reading: 0 Writing: 1 Waiting: 0
	var re *regexp.Regexp
	scanner := bufio.NewScanner(res.Body)
	defer res.Body.Close()

	// Parse active connections.
	scanner.Scan()
	re = regexp.MustCompile("Active connections: (\\d+)")
	var active int
	if matches := re.FindStringSubmatch(scanner.Text()); matches == nil {
		logp.Err("Fail to parse active connections from Nginx stub status")
		active = -1
	} else {
		active, _ = strconv.Atoi(matches[1])
	}

	// Skip request status headers.
	scanner.Scan()

	// Parse request status.
	scanner.Scan()
	re = regexp.MustCompile("\\s(\\d+)\\s+(\\d+)\\s+(\\d+)")
	var (
		accepts  int
		handled  int
		requests int
	)
	if matches := re.FindStringSubmatch(scanner.Text()); matches == nil {
		logp.Err("Fail to parse request status from Nginx stub status")
		accepts = -1
		handled = -1
		requests = -1
	} else {
		accepts, _ = strconv.Atoi(matches[1])
		handled, _ = strconv.Atoi(matches[2])
		requests, _ = strconv.Atoi(matches[3])
	}

	// Parse connection status.
	scanner.Scan()
	re = regexp.MustCompile("Reading: (\\d+) Writing: (\\d+) Waiting: (\\d+)")
	var (
		reading int
		writing int
		waiting int
	)
	if matches := re.FindStringSubmatch(scanner.Text()); matches == nil {
		logp.Err("Fail to parse connection status from Nginx stub status")
		reading = -1
		writing = -1
		waiting = -1
	} else {
		reading, _ = strconv.Atoi(matches[1])
		writing, _ = strconv.Atoi(matches[2])
		waiting, _ = strconv.Atoi(matches[3])
	}

	event := common.MapStr{
		"timestamp": common.Time(time.Now()),
		"type":      "nginx",
		"nginx": common.MapStr{
			"active":   active,
			"accepts":  accepts,
			"handled":  handled,
			"requests": requests,
			"reading":  reading,
			"writing":  writing,
			"waiting":  waiting,
		},
	}
	nb.events.PublishEvent(event)
}
