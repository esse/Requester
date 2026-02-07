package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

func TestMockServer_ReturnsRecordedResponse(t *testing.T) {
	outgoing := []snapshot.OutgoingRequest{
		{
			Method: "POST",
			URL:    "/send",
			Response: &snapshot.Response{
				Status: 200,
				Body:   map[string]any{"sent": true},
			},
		},
	}

	server := NewServer(outgoing)
	addr, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	resp, err := http.Post("http://"+addr+"/send", "application/json", strings.NewReader(`{"to":"test@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["sent"] != true {
		t.Errorf("expected sent=true, got %v", parsed["sent"])
	}

	calls := server.Calls()
	if len(calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(calls))
	}
}

func TestMockServer_UnmatchedRequest(t *testing.T) {
	server := NewServer(nil)
	addr, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	resp, err := http.Get("http://" + addr + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 for unmatched request, got %d", resp.StatusCode)
	}
}
