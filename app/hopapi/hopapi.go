// Package hopapi is de leesbril op het HOP-cluster: een kleine client voor
// de /v1-API van de leader (elke agent proxyt die door, dus HOP_ADDR mag naar
// elke agent wijzen). Eigen JSON-types met precies de velden die we tonen —
// geen dependency op de hop-module, dus host-testbaar en tamago-compatibel.
//
// Het transport is apphttp (HopOS) en niet net/http: het cluster praat plain
// http op het interne net, en net/http linkt onvoorwaardelijk crypto/tls mee —
// ~2,9MB die taskman en launcher anders in élk image meedragen voor TLS dat ze
// nooit gebruiken. Zie de pakket-doc van hop-os/metal/app/applib/apphttp.
//
// Auth is HOP's HMAC-schema (hop/pkg/httputil): X-Hop-Auth =
// hex(HMAC-SHA256(key, METHOD\nPATH\nhex(sha256(body)))). Lege key = geen
// auth (dev/standalone).
package hopapi

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/xinix00/HopOS/metal/app/applib/apphttp"
)

// Agent is één geregistreerde node (GET /v1/agents).
type Agent struct {
	ID       string    `json:"id"`
	Endpoint string    `json:"endpoint"`
	Version  string    `json:"version"`
	LastSeen time.Time `json:"last_seen"`
}

// Job is de gewenste staat van één job (GET /v1/jobs) — alleen de velden
// die de taskmanager toont.
type Job struct {
	Name    string            `json:"name"`
	Driver  string            `json:"driver,omitempty"` // "" = exec
	Image   string            `json:"image,omitempty"`
	Command string            `json:"command,omitempty"`
	Count   int               `json:"count,omitempty"` // 0 = 1
	Ports   map[string]int    `json:"ports,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
}

// Task is één draaiende instantie (GET /v1/jobs/{name}/status).
type Task struct {
	ID           string         `json:"id"`
	JobName      string         `json:"job_name"`
	Driver       string         `json:"driver"`
	Ports        map[string]int `json:"ports"`
	Pid          int            `json:"pid"`
	State        string         `json:"state"` // running/stopping/failed/stopped
	StartedAt    time.Time      `json:"started_at"`
	RestartCount int            `json:"restart_count"`
	CPUPercent   float64        `json:"cpu_percent"`
	MemPercent   float64        `json:"mem_percent"`
}

// Status is het clusteroverzicht (GET /v1/status).
type Status struct {
	ClusterName string         `json:"cluster_name"`
	Agents      int            `json:"agents"`
	Jobs        int            `json:"jobs"`
	TotalPlaced int            `json:"total_placed"`
	Settling    bool           `json:"settling"`
	Placed      map[string]int `json:"placed"` // jobnaam → geplaatst aantal
}

// JobStatus is het antwoord van GET /v1/jobs/{name}/status: welke agents de
// job dragen en per agent-id de tasks.
type JobStatus struct {
	Agents       []Agent           `json:"agents"`
	TasksByAgent map[string][]Task `json:"tasks_by_agent"`
}

// Client praat met één HOP-endpoint. Base is "http://host:poort" (zonder
// slash op het eind), Key de cluster-API-key ("" = geen auth).
type Client struct {
	Base string
	Key  string
}

// callTimeout is de totaaltermijn van een gewone call: een agent die niet
// antwoordt mag de UI-lus niet gijzelen. De logstaart staat er bewust búíten —
// die hóórt open te blijven.
const callTimeout = 10 * time.Second

// call doet één gesigneerd verzoek. De aanroeper sluit resp.Body.
func (c *Client) call(method, path string, body []byte, timeout time.Duration) (*apphttp.Response, error) {
	req := apphttp.Call{
		Method:  method,
		URL:     c.Base + path,
		Body:    body,
		Timeout: timeout,
	}
	if body != nil {
		req.Header = apphttp.Header{"Content-Type": "application/json"}
	}
	if c.Key != "" {
		if req.Header == nil {
			req.Header = apphttp.Header{}
		}
		req.Header.Set("X-Hop-Auth", sign(c.Key, method, path, body))
	}
	return apphttp.Do(req)
}

// fout maakt van een niet-2xx-antwoord een leesbare fout; de body zegt vaak
// wat er mis is, dus de eerste 200 bytes gaan mee.
func fout(wat string, resp *apphttp.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return fmt.Errorf("hop: %s: %s (%s)", wat, resp.Status, b)
}

// Status haalt het clusteroverzicht op.
func (c *Client) Status() (Status, error) {
	var s Status
	err := c.get("/v1/status", &s)
	return s, err
}

// Agents haalt de geregistreerde agents op.
func (c *Client) Agents() ([]Agent, error) {
	var a []Agent
	err := c.get("/v1/agents", &a)
	return a, err
}

// Jobs haalt alle jobs op.
func (c *Client) Jobs() ([]Job, error) {
	var j []Job
	err := c.get("/v1/jobs", &j)
	return j, err
}

// JobStatus haalt de tasks van één job op.
func (c *Client) JobStatus(name string) (JobStatus, error) {
	var js JobStatus
	err := c.get("/v1/jobs/"+name+"/status", &js)
	return js, err
}

// Apply dient een jobspec in (POST /v1/jobs — upsert, zoals `hop apply`).
// spec is de rauwe JSON: de launcher stuurt zijn catalogusregels
// onaangeroerd door, dus elke jobspec die de API begrijpt werkt hier ook.
func (c *Client) Apply(spec []byte) error {
	resp, err := c.call("POST", "/v1/jobs", spec, callTimeout)
	if err != nil {
		return fmt.Errorf("hop: apply: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != apphttp.StatusOK && resp.StatusCode != apphttp.StatusCreated {
		return fout("apply", resp)
	}
	return nil
}

// Delete verwijdert een job (DELETE /v1/jobs/{name}) — de stop-knop van de
// launcher: HOP ruimt de tasks op en het window verdwijnt vanzelf.
func (c *Client) Delete(name string) error {
	resp, err := c.call("DELETE", "/v1/jobs/"+name, nil, callTimeout)
	if err != nil {
		return fmt.Errorf("hop: delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != apphttp.StatusOK && resp.StatusCode != apphttp.StatusNoContent {
		return fout("delete", resp)
	}
	return nil
}

// LogStream is één live logstaart. Lines levert de regels; het kanaal sluit
// als de stream eindigt (task weg, verbinding stuk). Close stopt de stream.
type LogStream struct {
	Lines <-chan string

	body io.Closer
	stop chan struct{}
	once sync.Once
}

// Close stopt de stream; Lines sluit daarna vanzelf. De verbinding sluiten ís
// het afbreek-signaal: de pomp hangt in een Read en komt eruit met een fout.
func (s *LogStream) Close() {
	s.once.Do(func() {
		close(s.stop)
		s.body.Close()
	})
}

// Logs opent de live logstaart van één task (SSE: regels `data: <regel>`).
// stream is "stdout" of "stderr"; agentID en taskID komen uit JobStatus.
func (c *Client) Logs(agentID, taskID, stream string) (*LogStream, error) {
	path := "/v1/agents/" + agentID + "/logs/" + taskID + "/" + stream
	// Geen totaaltermijn: een logstaart is bedoeld om open te blijven.
	resp, err := c.call("GET", path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("hop: logs: %w", err)
	}
	if resp.StatusCode != apphttp.StatusOK {
		defer resp.Body.Close()
		return nil, fout("logs", resp)
	}

	ch := make(chan string, 64)
	ls := &LogStream{Lines: ch, body: resp.Body, stop: make(chan struct{})}
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 4096), 256*1024) // logregels kunnen fors zijn
		for sc.Scan() {
			line, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue // event-regels en de lege scheiders
			}
			select {
			case ch <- line:
			case <-ls.stop:
				return
			}
		}
	}()
	return ls, nil
}

// get doet een gesigneerde GET en decodeert JSON in out.
func (c *Client) get(path string, out any) error {
	resp, err := c.call("GET", path, nil, callTimeout)
	if err != nil {
		// Het pad ervoor, de oorzaak erachter: op de statusregel van taskman
		// moet dit in één oogopslag leesbaar zijn (Derek 19-07 — de fout liep
		// onleesbaar door de voetregel heen).
		return fmt.Errorf("hop: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != apphttp.StatusOK {
		return fout(path, resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// sign bouwt HOP's request-handtekening: HMAC over METHOD\nPATH\nbody-hash.
// Moet byte-voor-byte gelijk zijn aan hop/pkg/httputil.Sign.
func sign(key, method, path string, body []byte) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(method + "\n" + path + "\n" + hex.EncodeToString(sum[:])))
	return hex.EncodeToString(mac.Sum(nil))
}
