package nex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/PretendoNetwork/nex-go/v2/types"
	nexhost "github.com/openpak/nex"
	emit "github.com/openpak/rewards-emit"
)

func TestSplatfestMatchEmitsRewardsEvents(t *testing.T) {
	db := testDB(t)
	s := festStore{db: db}
	if err := s.setup(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var in struct{ Events []map[string]any }
		json.Unmarshal(b, &in)
		mu.Lock()
		got = append(got, in.Events...)
		mu.Unlock()
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := emit.New(emit.Config{URL: srv.URL, Key: "k", Source: "nex-splatoon", Flush: 10 * time.Millisecond})
	nexhost.UseRewards(c)

	p := NewCompetitionRankingUploadScoreParam()
	p.FestID, p.TeamID, p.Score, p.TeamScore, p.IsFirstUpload = 31, 1, 200, 1, types.Bool(true)
	if err := s.record(1234, p); err != nil {
		t.Fatal(err)
	}
	// A rejected upload (bad team) must emit nothing.
	bad := p
	bad.TeamID = 9
	if err := s.record(1234, bad); err == nil {
		t.Fatal("bad team accepted")
	}
	c.Close(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("events %v", got)
	}
	metrics := map[string]bool{}
	for _, e := range got {
		metrics[e["metric"].(string)] = true
		if id := e["identity"].(map[string]any); id["namespace"] != "wiiu" || id["subject"] != "1234" {
			t.Fatalf("identity %v", id)
		}
	}
	if !metrics["splatfests_joined"] {
		t.Fatalf("%v", metrics)
	}
}
