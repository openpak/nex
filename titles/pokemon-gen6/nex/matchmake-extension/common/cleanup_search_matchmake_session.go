package nex_matchmake_extension_common

import (
	"fmt"
	"github.com/openpak/nex/redact"

	matchmaking_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
)

func CleanupSearchMatchmakeSession(matchmakeSession *matchmaking_types.MatchmakeSession) {
	if redact.TraceBodies {
		fmt.Println(matchmakeSession)
	}
}
