package mk7

import (
	"sync"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/mario-kart-7/database"
	"github.com/openpak/nex/titles/mario-kart-7/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	nex.StartAPI(database.Postgres)
	nexhost.ServeRankings(nexhost.TimeTrialRankings(database.Postgres, "mk7.ranking_scores", "scores[1]"))
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
