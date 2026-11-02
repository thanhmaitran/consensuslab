package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func do(t *testing.T, ts *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := New(Options{AllowedOrigins: []string{"http://localhost:5173"}})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	return ts
}

func TestClusterLifecycle(t *testing.T) {
	ts := newTestServer(t)
	code, out := do(t, ts, "POST", "/api/v1/clusters", `{"node_count":3,"seed":7}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	id := out["id"].(string)

	if code, _ := do(t, ts, "POST", "/api/v1/clusters/"+id+"/step", `{"duration_ms":2000}`); code != http.StatusOK {
		t.Fatalf("step: %d", code)
	}
	_, out = do(t, ts, "GET", "/api/v1/clusters/"+id, "")
	status := out["status"].(map[string]any)
	if status["leader"] == "" || status["nodeCount"].(float64) != 3 {
		t.Fatalf("status = %v", status)
	}

	if code, _ := do(t, ts, "POST", "/api/v1/clusters/"+id+"/commands", `{"command":"SET a 1"}`); code != http.StatusAccepted {
		t.Fatalf("command: %d", code)
	}
	if code, _ := do(t, ts, "POST", "/api/v1/clusters/"+id+"/commands", `{"command":"nonsense"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid command: %d", code)
	}
	if code, _ := do(t, ts, "GET", "/api/v1/clusters/"+id+"/events?limit=5", ""); code != http.StatusOK {
		t.Fatalf("events: %d", code)
	}
	_, rep := do(t, ts, "GET", "/api/v1/clusters/"+id+"/report", "")
	if rep["status"] != "passed" {
		t.Fatalf("report = %v", rep)
	}
}

func TestNodeFaultErrors(t *testing.T) {
	ts := newTestServer(t)
	_, out := do(t, ts, "POST", "/api/v1/clusters", `{"node_count":3}`)
	id := out["id"].(string)
	base := "/api/v1/clusters/" + id + "/nodes/"
	if code, _ := do(t, ts, "POST", base+"node-9/crash", ""); code != http.StatusNotFound {
		t.Errorf("unknown node: %d", code)
	}
	if code, _ := do(t, ts, "POST", base+"node-1/restart", ""); code != http.StatusConflict {
		t.Errorf("restart of a running node: %d", code)
	}
	if code, _ := do(t, ts, "POST", base+"node-1/crash", ""); code != http.StatusOK {
		t.Errorf("crash: %d", code)
	}
	if code, _ := do(t, ts, "POST", base+"node-1/crash", ""); code != http.StatusConflict {
		t.Errorf("double crash: %d", code)
	}
}

func TestPartitionEndpoints(t *testing.T) {
	ts := newTestServer(t)
	_, out := do(t, ts, "POST", "/api/v1/clusters", `{"node_count":5}`)
	id := out["id"].(string)
	p := "/api/v1/clusters/" + id + "/partitions"
	if code, _ := do(t, ts, "POST", p, `{"groups":[["node-1"]]}`); code != http.StatusBadRequest {
		t.Errorf("single group: %d", code)
	}
	code, out := do(t, ts, "POST", p, `{"groups":[["node-1","node-2","node-3"],["node-4","node-5"]]}`)
	if code != http.StatusCreated {
		t.Fatalf("create partition: %d %v", code, out)
	}
	pid := out["partitionId"].(string)
	if code, _ := do(t, ts, "DELETE", p+"/nope", ""); code != http.StatusNotFound {
		t.Errorf("wrong partition id: %d", code)
	}
	if code, _ := do(t, ts, "DELETE", p+"/"+pid, ""); code != http.StatusOK {
		t.Errorf("heal: %d", code)
	}
}

func TestValidationAndLimits(t *testing.T) {
	ts := newTestServer(t)
	if code, _ := do(t, ts, "POST", "/api/v1/clusters", `{"node_count":99}`); code != http.StatusBadRequest {
		t.Errorf("99 nodes: %d", code)
	}
	if code, _ := do(t, ts, "POST", "/api/v1/clusters", `{"unknown":1}`); code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", code)
	}
	if code, _ := do(t, ts, "GET", "/api/v1/clusters/nope", ""); code != http.StatusNotFound {
		t.Errorf("missing cluster: %d", code)
	}
	big := `{"scenario":{"id":"X","node_counts":[3],"seeds":500,"duration_ms":1000}}`
	if code, _ := do(t, ts, "POST", "/api/v1/experiments", big); code != http.StatusBadRequest {
		t.Errorf("oversized experiment: %d", code)
	}
}

func TestStepRejectedWhileRunning(t *testing.T) {
	ts := newTestServer(t)
	_, out := do(t, ts, "POST", "/api/v1/clusters", `{"node_count":3}`)
	id := out["id"].(string)
	do(t, ts, "POST", "/api/v1/clusters/"+id+"/start", `{}`)
	if code, _ := do(t, ts, "POST", "/api/v1/clusters/"+id+"/step", `{}`); code != http.StatusConflict {
		t.Errorf("step while running: %d", code)
	}
	if code, _ := do(t, ts, "POST", "/api/v1/clusters/"+id+"/pause", ""); code != http.StatusOK {
		t.Errorf("pause: %d", code)
	}
}

func TestCORSAllowList(t *testing.T) {
	ts := newTestServer(t)
	for origin, want := range map[string]string{
		"http://localhost:5173": "http://localhost:5173",
		"http://evil.example":   "",
	} {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/health", nil)
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("origin %s: header %q, want %q", origin, got, want)
		}
	}
}
