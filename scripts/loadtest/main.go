// Command loadtest submits a batch of jobs to a running gpu-runner server,
// waits for all of them to finish, and reports throughput, queue wait time,
// run time and per-GPU busy time.
//
// It measures timings from each job's log stream: created ("Successfully
// created job!"), started ("Job Running") and finished ("Completed job").
// Use run.sh to start a server against fake GPUs and run this against it.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type job struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Log    string `json:"log"`
	Error  string `json:"error"`
}

type logEntry struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	server := flag.String("server", "http://127.0.0.1:18080", "gpu-runner server URL")
	numJobs := flag.Int("jobs", 200, "number of jobs to submit")
	seed := flag.Int64("seed", 1, "random seed for job sizes and durations")
	minRun := flag.Duration("min-run", 500*time.Millisecond, "shortest job run time")
	maxRun := flag.Duration("max-run", 2*time.Second, "longest job run time")
	largeShare := flag.Float64("large", 0.3, "share of large jobs (30-60 GB); the rest are 4-20 GB")
	timeout := flag.Duration("timeout", 15*time.Minute, "give up waiting after this long")
	readyFile := flag.String("ready-file", "", "write this file once all jobs are submitted")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	client := &http.Client{Timeout: 10 * time.Second}

	// Sample GPU busy state from /metrics while the test runs.
	sampler := newBusySampler(client, *server)
	stopSampling := make(chan struct{})
	samplingDone := make(chan struct{})
	go func() { sampler.run(200*time.Millisecond, stopSampling); close(samplingDone) }()

	fmt.Printf("Submitting %d jobs to %s ...\n", *numJobs, *server)
	ids := make([]string, 0, *numJobs)
	sizes := make(map[string]int, *numJobs)
	for i := 0; i < *numJobs; i++ {
		memMB := 4000 + rng.Intn(16001) // 4-20 GB
		if rng.Float64() < *largeShare {
			memMB = 30000 + rng.Intn(30001) // 30-60 GB
		}
		run := *minRun + time.Duration(rng.Int63n(int64(*maxRun-*minRun)+1))
		// A unique process name lets run.sh clean up only these jobs after a crash.
		id, err := submit(client, *server, fmt.Sprintf("exec -a gpurunner-loadtest sleep %.3f", run.Seconds()), memMB)
		if err != nil {
			fatalf("submit job %d: %v", i, err)
		}
		ids = append(ids, id)
		sizes[id] = memMB
	}
	fmt.Printf("Submitted %d jobs.\n", len(ids))
	if *readyFile != "" {
		if err := os.WriteFile(*readyFile, []byte("ready\n"), 0o644); err != nil {
			fatalf("write ready file: %v", err)
		}
	}

	if err := waitForAll(client, *server, ids, *timeout); err != nil {
		fatalf("%v", err)
	}
	close(stopSampling)
	<-samplingDone

	report(client, *server, ids, sizes, sampler)
}

func submit(client *http.Client, server, command string, memMB int) (string, error) {
	body, _ := json.Marshal(map[string]any{"command": command, "memory_mb": memMB, "max_retries": 1})
	resp, err := client.Post(server+"/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var j job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return "", err
	}
	return j.ID, nil
}

func terminal(status string) bool {
	return status == "success" || status == "failed" || status == "cancelled"
}

// waitForAll polls the job list until every submitted job has finished. It
// tolerates the server being briefly unreachable (e.g. a crash test restart).
func waitForAll(client *http.Client, server string, ids []string, timeout time.Duration) error {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	deadline := time.Now().Add(timeout)
	lastPrint := time.Time{}
	for time.Now().Before(deadline) {
		var list []job
		if err := getJSON(client, server+"/jobs", &list); err == nil {
			done := 0
			for _, j := range list {
				if want[j.ID] && terminal(j.Status) {
					done++
				}
			}
			if done == len(ids) {
				return nil
			}
			if time.Since(lastPrint) > 5*time.Second {
				fmt.Printf("  %d/%d jobs finished\n", done, len(ids))
				lastPrint = time.Now()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s waiting for jobs to finish", timeout)
}

func getJSON(client *http.Client, url string, out any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func report(client *http.Client, server string, ids []string, sizes map[string]int, sampler *busySampler) {
	statuses := map[string]int{}
	var waits, runs []time.Duration
	var first, last time.Time
	var reruns int
	for _, id := range ids {
		var j job
		if err := getJSON(client, server+"/jobs/"+id, &j); err != nil {
			fatalf("get job %s: %v", id, err)
		}
		statuses[j.Status]++

		var entries []logEntry
		_ = json.Unmarshal([]byte(j.Log), &entries)
		var created, started, lastStart, finished time.Time
		starts := 0
		for _, e := range entries {
			switch {
			case e.Message == "Successfully created job!":
				created = e.Timestamp
			case strings.HasPrefix(e.Message, "Job Running"):
				starts++
				if started.IsZero() {
					started = e.Timestamp
				}
				lastStart = e.Timestamp
			case strings.HasPrefix(e.Message, "Completed job"):
				finished = e.Timestamp
			}
		}
		if starts > 1 {
			reruns++
		}
		if !created.IsZero() && !started.IsZero() {
			waits = append(waits, started.Sub(created))
		}
		if !lastStart.IsZero() && !finished.IsZero() {
			runs = append(runs, finished.Sub(lastStart))
		}
		if !created.IsZero() && (first.IsZero() || created.Before(first)) {
			first = created
		}
		if finished.After(last) {
			last = finished
		}
	}

	small, large := 0, 0
	for _, mb := range sizes {
		if mb >= 30000 {
			large++
		} else {
			small++
		}
	}

	fmt.Println()
	fmt.Println("==================== Load test results ====================")
	fmt.Printf("Jobs submitted:     %d (%d small 4-20 GB, %d large 30-60 GB)\n", len(ids), small, large)
	fmt.Printf("Outcomes:           %s\n", formatCounts(statuses))
	lost := len(ids) - statuses["success"] - statuses["failed"] - statuses["cancelled"]
	fmt.Printf("Lost jobs:          %d\n", lost)
	if reruns > 0 {
		fmt.Printf("Re-run after crash: %d (started more than once)\n", reruns)
	}
	if !first.IsZero() && last.After(first) {
		total := last.Sub(first)
		fmt.Printf("Total time:         %s\n", total.Round(10*time.Millisecond))
		fmt.Printf("Throughput:         %.1f jobs/min\n", float64(statuses["success"])/total.Minutes())
	}
	fmt.Printf("Queue wait:         p50 %s, p95 %s, max %s\n", pct(waits, 50), pct(waits, 95), pct(waits, 100))
	fmt.Printf("Run time:           p50 %s, p95 %s\n", pct(runs, 50), pct(runs, 95))
	if gpus := sampler.busyShares(); len(gpus) > 0 {
		fmt.Println("GPU busy time (sampled every 200ms):")
		for _, g := range gpus {
			fmt.Printf("  %-10s %5.1f%%\n", g.gpu, g.share*100)
		}
	}
	fmt.Println("===========================================================")
}

func formatCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

// pct returns the p-th percentile (nearest rank) of ds.
func pct(ds []time.Duration, p int) string {
	if len(ds) == 0 {
		return "n/a"
	}
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	i := (p*len(sorted)+99)/100 - 1
	if i < 0 {
		i = 0
	}
	return sorted[i].Round(time.Millisecond).String()
}

// busySampler scrapes gpu_runner_gpu_busy from /metrics and counts, per GPU,
// how many samples saw it busy.
type busySampler struct {
	client  *http.Client
	url     string
	mu      sync.Mutex
	samples int
	busy    map[string]int
}

type gpuShare struct {
	gpu   string
	share float64
}

func newBusySampler(client *http.Client, server string) *busySampler {
	return &busySampler{client: client, url: server + "/metrics", busy: map[string]int{}}
}

func (s *busySampler) run(every time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.sample()
		}
	}
}

func (s *busySampler) sample() {
	resp, err := s.client.Get(s.url)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()

	seen := map[string]bool{}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, `gpu_runner_gpu_busy{gpu="`) {
			continue
		}
		rest := strings.TrimPrefix(line, `gpu_runner_gpu_busy{gpu="`)
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		gpu := rest[:end]
		val, err := strconv.ParseFloat(strings.TrimSpace(rest[strings.LastIndex(rest, " ")+1:]), 64)
		if err != nil {
			continue
		}
		seen[gpu] = val == 1
	}
	if len(seen) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples++
	for gpu, busy := range seen {
		if busy {
			s.busy[gpu]++
		} else if _, ok := s.busy[gpu]; !ok {
			s.busy[gpu] = 0
		}
	}
}

func (s *busySampler) busyShares() []gpuShare {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.samples == 0 {
		return nil
	}
	out := make([]gpuShare, 0, len(s.busy))
	for gpu, n := range s.busy {
		out = append(out, gpuShare{gpu, float64(n) / float64(s.samples)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].gpu < out[j].gpu })
	return out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadtest: "+format+"\n", args...)
	os.Exit(1)
}
