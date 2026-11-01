package nex

import (
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonmatchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	commonmatchmakingext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	commonmatchmakeextension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	commonnattraversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	commonranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	commonsecure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nattraversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/splatoon"
	secure "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"
	"github.com/openpak/nex/titles/splatoon/globals"
)

func CreateReportDBRecord(_ types.PID, _ types.UInt32, _ types.QBuffer) error {
	return nil
}

func registerCommonSecureServerProtocols() {
	secureProtocol := secure.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(secureProtocol)
	commonSecureProtocol := commonsecure.NewCommonProtocol(secureProtocol)
	commonSecureProtocol.EnableInsecureRegister()
	commonSecureProtocol.CreateReportDBRecord = CreateReportDBRecord

	natTraversalProtocol := nattraversal.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(natTraversalProtocol)
	commonnattraversal.NewCommonProtocol(natTraversalProtocol)

	matchMakingProtocol := matchmaking.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingProtocol)
	commonMatchMakingProtocol := commonmatchmaking.NewCommonProtocol(matchMakingProtocol)
	commonMatchMakingProtocol.SetManager(globals.MatchmakingManager)

	matchMakingExtProtocol := matchmakingext.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingExtProtocol)
	commonMatchMakingExtProtocol := commonmatchmakingext.NewCommonProtocol(matchMakingExtProtocol)
	commonMatchMakingExtProtocol.SetManager(globals.MatchmakingManager)

	matchmakeExtensionProtocol := matchmakeextension.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol := commonmatchmakeextension.NewCommonProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol.CleanupMatchmakeSessionSearchCriterias = cleanupMatchmakeSessionSearchCriteriasHandler
	commonMatchmakeExtensionProtocol.OnAfterAutoMatchmakeWithParamPostpone = onAfterAutoMatchmakeWithParamPostpone
	commonMatchmakeExtensionProtocol.SetManager(globals.MatchmakingManager)

	rankingProtocol := ranking.NewProtocol(globals.SecureEndpoint)
	globals.SecureEndpoint.RegisterServiceProtocol(rankingProtocol)
	commonranking.NewCommonProtocol(rankingProtocol)
	fests := festStore{db: globals.Postgres}
	if err := fests.setup(); err != nil {
		globals.Logger.Errorf("Ranking: Splatfest tables: %v", err)
	}
	fests.wire(rankingProtocol)
	startRotation()
}
