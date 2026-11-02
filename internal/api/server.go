// Package api exposes the simulation engine over REST and a Server-Sent
// Events stream. The frontend can only issue requests; it never mutates Raft
// state directly.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/experiment"
	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/raft"
	"github.com/thanhmaitran/consensuslab/internal/sim"
)

// Limits that protect a public demo from unbounded workloads.
const (
	maxClusters           = 16
	maxExperimentsStored  = 20
	maxRunningExperiments = 2
	maxStepEvents         = 10000
	maxStepDurationMs     = 60000
	maxExperimentSeeds    = 100
	maxExperimentRuns     = 300
	maxExperimentMs       = 60000
	maxExperimentCmds     = 500
	tickWall              = 50 * time.Millisecond
)

// Options configures the HTTP server.
type Options struct {
	AllowedOrigins []string // CORS allow-list; empty disables CORS headers
	StaticDir      string   // optional directory with the built dashboard
	CodeVersion    string
}

type cluster struct {
	id          string
	mu          sync.Mutex
	eng         *sim.Engine
	running     bool
	speed       float64
	cancel      context.CancelFunc
	partitionID string
	partSeq     int
}

type experimentJob struct {
	ID         string
	Status     string // running | done | error
	Done       int
	Total      int
	Err        string
	Aggregates []experiment.Aggregate
	Records    []experiment.RunRecord
	Violations int
}

// Server holds all clusters and experiments.
type Server struct {
	mu          sync.Mutex
	clusters    map[string]*cluster
	experiments map[string]*experimentJob
	expOrder    []string
	clusterSeq  int
	expSeq      int
	opts        Options
	mux         *http.ServeMux
}

// New builds the server and registers its routes.
func New(opts Options) *Server {
	s := &Server{
		clusters:    make(map[string]*cluster),
		experiments: make(map[string]*experimentJob),
		opts:        opts,
		mux:         http.NewServeMux(),
	}
	const p = "/api/v1"
	s.mux.HandleFunc("GET "+p+"/health", s.handleHealth)
	s.mux.HandleFunc("GET "+p+"/clusters", s.handleListClusters)
	s.mux.HandleFunc("POST "+p+"/clusters", s.handleCreateCluster)
	s.mux.HandleFunc("GET "+p+"/clusters/{id}", s.handleGetCluster)
	s.mux.HandleFunc("DELETE "+p+"/clusters/{id}", s.handleDeleteCluster)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/start", s.handleStart)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/pause", s.handlePause)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/step", s.handleStep)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/nodes/{nodeId}/crash", s.handleCrash)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/nodes/{nodeId}/restart", s.handleRestart)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/partitions", s.handleCreatePartition)
	s.mux.HandleFunc("DELETE "+p+"/clusters/{id}/partitions/{partitionId}", s.handleDeletePartition)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/commands", s.handleCommand)
	s.mux.HandleFunc("POST "+p+"/clusters/{id}/network", s.handleNetwork)
	s.mux.HandleFunc("GET "+p+"/clusters/{id}/events", s.handleEvents)
	s.mux.HandleFunc("GET "+p+"/clusters/{id}/stream", s.handleStream)
	s.mux.HandleFunc("GET "+p+"/clusters/{id}/report", s.handleReport)
	s.mux.HandleFunc("POST "+p+"/experiments", s.handleCreateExperiment)
	s.mux.HandleFunc("GET "+p+"/experiments/{id}", s.handleGetExperiment)
	if opts.StaticDir != "" {
		s.mux.Handle("GET /", http.FileServer(http.Dir(opts.StaticDir)))
	}
	return s
}

// Handler returns the root handler including CORS handling.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			for _, allowed := range s.opts.AllowedOrigins {
				if allowed == origin {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Vary", "Origin")
					h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type")
					break
				}
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

// Close stops every running simulation loop.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clusters {
		c.mu.Lock()
		c.running = false
		if c.cancel != nil {
			c.cancel()
		}
		c.mu.Unlock()
	}
}

// ------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeEngineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sim.ErrUnknownNode):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// decode reads a JSON body (empty bodies keep the defaults in v).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*cluster, bool) {
	id := r.PathValue("id")
	s.mu.Lock()
	c, ok := s.clusters[id]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("cluster %q not found", id))
		return nil, false
	}
	return c, true
}

func (c *cluster) view() map[string]any {
	return map[string]any{
		"id": c.id, "running": c.running, "speed": c.speed,
		"partitionId": c.partitionID, "status": c.eng.Status(),
	}
}

// ------------------------------------------------------------- clusters

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.opts.CodeVersion})
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.clusters))
	for id := range s.clusters {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"clusters": ids})
}

type createClusterRequest struct {
	NodeCount         int                     `json:"node_count"`
	Seed              *int64                  `json:"seed"`
	Network           *experiment.NetworkJSON `json:"network"`
	ElectionTimeoutMs *[2]float64             `json:"election_timeout_ms"`
	HeartbeatMs       float64                 `json:"heartbeat_ms"`
	MaxVirtualTimeMs  float64                 `json:"max_virtual_time_ms"`
	VerboseEvents     bool                    `json:"verbose_events"`
}

func (s *Server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	var req createClusterRequest
	if !decode(w, r, &req) {
		return
	}
	cfg := sim.DefaultConfig()
	if req.NodeCount != 0 {
		cfg.NodeCount = req.NodeCount
	}
	if req.Seed != nil {
		cfg.Seed = *req.Seed
	}
	if req.Network != nil {
		cfg.Network = req.Network.Config()
	}
	if req.ElectionTimeoutMs != nil {
		cfg.ElectionTimeoutMin = time.Duration(req.ElectionTimeoutMs[0] * float64(time.Millisecond))
		cfg.ElectionTimeoutMax = time.Duration(req.ElectionTimeoutMs[1] * float64(time.Millisecond))
	}
	if req.HeartbeatMs != 0 {
		cfg.HeartbeatInterval = time.Duration(req.HeartbeatMs * float64(time.Millisecond))
	}
	if req.MaxVirtualTimeMs != 0 {
		if req.MaxVirtualTimeMs < 0 || req.MaxVirtualTimeMs > experiment.MaxDurationMs {
			writeError(w, http.StatusBadRequest, "max_virtual_time_ms is out of range")
			return
		}
		cfg.MaxVirtualTime = time.Duration(req.MaxVirtualTimeMs * float64(time.Millisecond))
	}
	cfg.VerboseEvents = req.VerboseEvents

	s.mu.Lock()
	if len(s.clusters) >= maxClusters {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "cluster limit reached; delete an existing cluster first")
		return
	}
	s.clusterSeq++
	id := "c" + strconv.Itoa(s.clusterSeq)
	s.mu.Unlock()

	cfg.ScenarioID = id
	eng, err := sim.NewEngine(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c := &cluster{id: id, eng: eng, speed: 1}
	s.mu.Lock()
	s.clusters[id] = c
	s.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	writeJSON(w, http.StatusCreated, c.view())
}

func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	writeJSON(w, http.StatusOK, c.view())
}

func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	c.mu.Lock()
	c.running = false
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	s.mu.Lock()
	delete(s.clusters, c.id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ------------------------------------------------------------- control

type startRequest struct {
	Speed float64 `json:"speed"`
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req startRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Speed == 0 {
		req.Speed = 1
	}
	if req.Speed < 0.1 || req.Speed > 20 {
		writeError(w, http.StatusBadRequest, "speed must be between 0.1 and 20")
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.speed = req.Speed
	if !c.running {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.running = true
		go c.loop(ctx)
	}
	writeJSON(w, http.StatusOK, c.view())
}

func (c *cluster) loop(ctx context.Context) {
	t := time.NewTicker(tickWall)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c.mu.Lock()
		if !c.running {
			c.mu.Unlock()
			return
		}
		before := c.eng.Now
		c.eng.RunFor(time.Duration(float64(tickWall) * c.speed))
		if c.eng.Now == before || c.eng.Now >= c.eng.Config().MaxVirtualTime {
			c.running = false
		}
		c.mu.Unlock()
	}
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	writeJSON(w, http.StatusOK, c.view())
}

type stepRequest struct {
	Events     int     `json:"events"`
	DurationMs float64 `json:"duration_ms"`
}

func (s *Server) handleStep(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req stepRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Events < 0 || req.Events > maxStepEvents || req.DurationMs < 0 || req.DurationMs > maxStepDurationMs {
		writeError(w, http.StatusBadRequest, "events or duration_ms is out of range")
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		writeError(w, http.StatusConflict, "pause the simulation before stepping it manually")
		return
	}
	processed := 0
	switch {
	case req.DurationMs > 0:
		processed = c.eng.RunFor(time.Duration(req.DurationMs * float64(time.Millisecond)))
	default:
		n := req.Events
		if n == 0 {
			n = 1
		}
		for i := 0; i < n && c.eng.Step(); i++ {
			processed++
		}
	}
	out := c.view()
	out["processed"] = processed
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCrash(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	id := raft.NodeID(r.PathValue("nodeId"))
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.eng.Nodes[id]; !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
		return
	}
	if c.eng.IsDown(id) {
		writeError(w, http.StatusConflict, fmt.Sprintf("node %q is already down", id))
		return
	}
	if err := c.eng.Crash(id); err != nil {
		writeEngineError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.view())
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	id := raft.NodeID(r.PathValue("nodeId"))
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.eng.Nodes[id]; !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", id))
		return
	}
	if !c.eng.IsDown(id) {
		writeError(w, http.StatusConflict, fmt.Sprintf("node %q is not down", id))
		return
	}
	if err := c.eng.Restart(id); err != nil {
		writeEngineError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.view())
}

type partitionRequest struct {
	Groups [][]string `json:"groups"`
}

func (s *Server) handleCreatePartition(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req partitionRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Groups) < 2 {
		writeError(w, http.StatusBadRequest, "a partition needs at least two groups")
		return
	}
	groups := make([][]raft.NodeID, len(req.Groups))
	for i, g := range req.Groups {
		for _, id := range g {
			groups[i] = append(groups[i], raft.NodeID(id))
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.eng.Partition(groups); err != nil {
		writeEngineError(w, err)
		return
	}
	c.partSeq++
	c.partitionID = "p" + strconv.Itoa(c.partSeq)
	out := c.view()
	out["partitionId"] = c.partitionID
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleDeletePartition(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.partitionID == "" || c.partitionID != r.PathValue("partitionId") || !c.eng.Network.Partitioned() {
		writeError(w, http.StatusNotFound, "no such active partition")
		return
	}
	c.eng.Heal()
	c.partitionID = ""
	writeJSON(w, http.StatusOK, c.view())
}

type commandRequest struct {
	Command string `json:"command"`
	NodeID  string `json:"node_id"`
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req commandRequest
	if !decode(w, r, &req) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id, err := c.eng.Submit(raft.NodeID(req.NodeID), req.Command)
	if err != nil {
		writeEngineError(w, err)
		return
	}
	out := c.view()
	out["commandId"] = id
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req experiment.NetworkJSON
	if !decode(w, r, &req) {
		return
	}
	cfg := network.FromMillis(req.BaseDelayMs, req.JitterMs, req.LossProbability)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.eng.SetNetwork(cfg); err != nil {
		writeEngineError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.view())
}

// ------------------------------------------------------------- observation

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	events := c.eng.Recorder.Since(since, limit)
	last := since
	if len(events) > 0 {
		last = events[len(events)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events, "lastId": last, "dropped": c.eng.Recorder.Dropped(),
	})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}
	last, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		c.mu.Lock()
		events := c.eng.Recorder.Since(last, 200)
		c.mu.Unlock()
		for _, ev := range events {
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\nevent: sim\ndata: %s\n\n", ev.ID, data)
			last = ev.ID
		}
		if len(events) == 0 {
			fmt.Fprint(w, ": keep-alive\n\n")
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	writeJSON(w, http.StatusOK, c.eng.Finish())
}

// ------------------------------------------------------------- experiments

type experimentRequest struct {
	Scenario experiment.Scenario `json:"scenario"`
}

func (s *Server) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	var req experimentRequest
	if !decode(w, r, &req) {
		return
	}
	sc := req.Scenario
	if err := sc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sc.Seeds > maxExperimentSeeds || sc.DurationMs > maxExperimentMs ||
		sc.Workload.Commands > maxExperimentCmds || len(sc.Plan()) > maxExperimentRuns {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"scenario exceeds API limits (seeds <= %d, duration <= %d ms, commands <= %d, total runs <= %d); use the CLI for larger experiments",
			maxExperimentSeeds, maxExperimentMs, maxExperimentCmds, maxExperimentRuns))
		return
	}

	s.mu.Lock()
	running := 0
	for _, j := range s.experiments {
		if j.Status == "running" {
			running++
		}
	}
	if running >= maxRunningExperiments {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "too many experiments are running; retry later")
		return
	}
	s.expSeq++
	job := &experimentJob{ID: "e" + strconv.Itoa(s.expSeq), Status: "running", Total: len(sc.Plan())}
	s.experiments[job.ID] = job
	s.expOrder = append(s.expOrder, job.ID)
	for len(s.expOrder) > maxExperimentsStored {
		delete(s.experiments, s.expOrder[0])
		s.expOrder = s.expOrder[1:]
	}
	s.mu.Unlock()

	go func() {
		out, err := experiment.RunAll(&sc, experiment.Options{
			Workers:     1,
			CodeVersion: s.opts.CodeVersion,
			Progress: func(done, total int) {
				s.mu.Lock()
				job.Done = done
				s.mu.Unlock()
			},
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			job.Status, job.Err = "error", err.Error()
			return
		}
		job.Status = "done"
		job.Aggregates = out.Aggregates
		job.Records = out.Records
		for _, rec := range out.Records {
			job.Violations += rec.SafetyViolations
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": job.Status, "total": job.Total})
}

func (s *Server) handleGetExperiment(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.experiments[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "experiment not found")
		return
	}
	runs := job.Records
	if len(runs) > 1000 {
		runs = runs[:1000]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": job.ID, "status": job.Status, "error": job.Err,
		"progress":         map[string]int{"done": job.Done, "total": job.Total},
		"aggregates":       job.Aggregates,
		"runs":             runs,
		"safetyViolations": job.Violations,
	})
}
