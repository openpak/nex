package globals

import (
	"database/sql"
	pbfriends "github.com/PretendoNetwork/grpc/go/friends"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/plogger-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var Logger *plogger.Logger
var KerberosPassword = "password" // * Default password

var AuthenticationServer *nex.PRUDPServer
var AuthenticationEndpoint *nex.PRUDPEndPoint

var SecureServer *nex.PRUDPServer
var SecureEndpoint *nex.PRUDPEndPoint

var Postgres *sql.DB
var MatchmakingManager *common_globals.MatchmakingManager
var UtilityManager *common_globals.UtilityManager

// CoreCandidates, when set (TERRARIA_CORE_SEARCH=1), answers the gathering ids the matchmaking core
// lists for this title and group; nil from it means no restriction.
var CoreCandidates func() []uint32

var GRPCFriendsClientConnection *grpc.ClientConn
var GRPCFriendsClient pbfriends.FriendsClient
var GRPCFriendsCommonMetadata metadata.MD
