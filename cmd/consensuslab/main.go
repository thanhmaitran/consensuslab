// Command consensuslab is the entry point for the ConsensusLab simulator.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/api"
	"github.com/thanhmaitran/consensuslab/internal/experiment"
)

// Version is overridden at build time with -ldflags "-X main.Version=...".
var Version = "dev"

const usage = `ConsensusLab - Distributed Systems Simulator

Usage:
  consensuslab simulate   -config FILE [-nodes N] [-variant NAME] [-seed S] [-events FILE]
  consensuslab replay     -config FILE -nodes N [-variant NAME] -seed S [-expect TRACE_HASH]
  consensuslab experiment -config FILE -out DIR [-workers N]
  consensuslab serve      [-addr :8080] [-origin URL]... [-static DIR]
  consensuslab version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return
	}
	var err error
	switch os.Args[1] {
	case "simulate":
		err = runSimulate(os.Args[2:])
	case "replay":
		err = runReplay(os.Args[2:])
	case "experiment":
		err = runExperiment(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// pickSpec selects one run of a scenario from command-line flags.
func pickSpec(s *experiment.Scenario, nodes int, variant string, seed int64, seedSet bool) (experiment.RunSpec, error) {
	if nodes == 0 {
		nodes = s.NodeCounts[0]
	}
	found := false
	for _, n := range s.NodeCounts {
		if n == nodes {
			found = true
		}
	}
	if !found {
		return experiment.RunSpec{}, fmt.Errorf("scenario %q does not define a %d-node configuration", s.ID, nodes)
	}
	variants := s.ResolveVariants()
	chosen := variants[0]
	if variant != "" {
		ok := false
		for _, v := range variants {
			if v.Name == variant {
				chosen, ok = v, true
			}
		}
		if !ok {
			return experiment.RunSpec{}, fmt.Errorf("unknown variant %q", variant)
		}
	}
	if !seedSet {
		seed = s.SeedBase
	}
	return experiment.RunSpec{NodeCount: nodes, Variant: chosen, Seed: seed}, nil
}

func seedWasSet(fs *flag.FlagSet) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "seed" {
			set = true
		}
	})
	return set
}

func runSimulate(args []string) error {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	config := fs.String("config", "", "scenario file (JSON)")
	nodes := fs.Int("nodes", 0, "node count (default: first in the scenario)")
	variant := fs.String("variant", "", "network variant name")
	seed := fs.Int64("seed", 0, "seed (default: scenario seed_base)")
	events := fs.String("events", "", "write the event trace to this JSONL file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *config == "" {
		return fmt.Errorf("-config is required")
	}
	s, err := experiment.Load(*config)
	if err != nil {
		return err
	}
	spec, err := pickSpec(s, *nodes, *variant, *seed, seedWasSet(fs))
	if err != nil {
		return err
	}
	res, err := experiment.RunOne(s, spec, *events != "", Version)
	if err != nil {
		return err
	}
	if *events != "" {
		f, err := os.Create(*events)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for _, ev := range res.Events {
			if err := enc.Encode(ev); err != nil {
				f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	out := map[string]any{"record": res.Record, "verifier": res.Report}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if res.Report.Status != "passed" {
		return fmt.Errorf("safety verification failed (%d violations)", len(res.Report.Violations))
	}
	return nil
}

func runReplay(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	config := fs.String("config", "", "scenario file (JSON)")
	nodes := fs.Int("nodes", 0, "node count")
	variant := fs.String("variant", "", "network variant name")
	seed := fs.Int64("seed", 0, "seed of the run to replay")
	expect := fs.String("expect", "", "expected trace hash (from runs.jsonl)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *config == "" {
		return fmt.Errorf("-config is required")
	}
	s, err := experiment.Load(*config)
	if err != nil {
		return err
	}
	spec, err := pickSpec(s, *nodes, *variant, *seed, true)
	if err != nil {
		return err
	}
	first, err := experiment.RunOne(s, spec, false, Version)
	if err != nil {
		return err
	}
	second, err := experiment.RunOne(s, spec, false, Version)
	if err != nil {
		return err
	}
	fmt.Println("trace hash:", first.Record.TraceHash)
	if first.Record.TraceHash != second.Record.TraceHash {
		return fmt.Errorf("NOT reproducible: two runs with the same seed produced different traces")
	}
	fmt.Println("self-check: two runs produced identical traces")
	if *expect != "" {
		if !strings.EqualFold(*expect, first.Record.TraceHash) {
			return fmt.Errorf("trace hash differs from the expected value %s", *expect)
		}
		fmt.Println("matches the expected trace hash")
	}
	return nil
}

func runExperiment(args []string) error {
	fs := flag.NewFlagSet("experiment", flag.ContinueOnError)
	config := fs.String("config", "", "scenario file (JSON)")
	out := fs.String("out", "", "output directory")
	workers := fs.Int("workers", 1, "parallel workers")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *config == "" || *out == "" {
		return fmt.Errorf("-config and -out are required")
	}
	s, err := experiment.Load(*config)
	if err != nil {
		return err
	}
	result, err := experiment.RunAll(s, experiment.Options{
		OutDir:      *out,
		Workers:     *workers,
		CodeVersion: Version,
		Progress: func(done, total int) {
			if done == total || done%10 == 0 {
				fmt.Fprintf(os.Stderr, "\r%d/%d runs", done, total)
			}
		},
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Printf("wrote results to %s (%d runs, %d with safety violations)\n", *out, len(result.Records), len(result.Failures))
	if len(result.Failures) > 0 {
		return fmt.Errorf("safety violations detected; see %s/failures", *out)
	}
	return nil
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "listen address")
	static := fs.String("static", "", "directory with the built dashboard (web/dist)")
	var origins multiFlag
	fs.Var(&origins, "origin", "allowed CORS origin (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(origins) == 0 {
		origins = multiFlag{"http://localhost:5173"}
	}
	srv := api.New(api.Options{AllowedOrigins: origins, StaticDir: *static, CodeVersion: Version})
	defer srv.Close()
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout: the SSE stream is long-lived.
	}
	fmt.Printf("ConsensusLab %s listening on %s (simulation only, not a production consensus service)\n", Version, *addr)
	return httpSrv.ListenAndServe()
}
