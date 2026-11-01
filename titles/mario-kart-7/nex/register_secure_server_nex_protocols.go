package nex

import (
	storage_manager "github.com/PretendoNetwork/nex-protocols-go/v2/storage-manager"
	"github.com/openpak/nex/titles/mario-kart-7/database"
	"github.com/openpak/nex/titles/mario-kart-7/globals"

	nex_storage_manager "github.com/openpak/nex/titles/mario-kart-7/nex/storage-manager"
)

func registerSecureServerNEXProtocols() {
	// Ghosts: DataStore metadata in Postgres, bodies in S3 (MK7_S3_*). No friend list is
	// wired on this title, so friend-scoped searches see no friends.
	if err := registerDataStore(globals.SecureEndpoint, database.Postgres, func(uint32) []uint32 { return nil }); err != nil {
		globals.Logger.Criticalf("DataStore: %v", err)
	}

	// Time-trial rankings: the legacy Ranking protocol (0x70), see ranking.go.
	globals.SecureEndpoint.RegisterServiceProtocol(registerLegacyRanking(database.Ranking{DB: database.Postgres}))

	storageManagerProtocol := storage_manager.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(storageManagerProtocol)

	storageManagerProtocol.AcquireCardID = nex_storage_manager.AcquireCardID
	storageManagerProtocol.ActivateWithCardID = nex_storage_manager.ActivateWithCardID
}
