package ranking

import (
	"fmt"
	"github.com/openpak/nex/redact"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
)

// Stubbed
func InsertRankingByPIDAndRankingScoreData(pid types.PID, rankingScoreData ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	if redact.TraceBodies {
		fmt.Println(rankingScoreData)
	}
	if redact.TraceBodies {
		fmt.Println(uniqueID)
	}
	if redact.TraceBodies {
		fmt.Println(pid)
	}

	return nil
}
