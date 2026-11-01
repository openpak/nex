package mk7

import (
	"sync"

	"github.com/openpak/nex/titles/mario-kart-7/database"
	"github.com/openpak/nex/titles/mario-kart-7/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	nex.StartAPI(database.Postgres)
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
