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

// IsCrossRoom, when set, reports whether a gathering id is a shadow of another network's room (cross-play listing).
var IsCrossRoom func(gid uint32) bool

// RelayStation is the relay's PIA address ("ip:port", TERRARIA_RELAY_STATION). When set, a Wii U may join another network's
// room: GetSessionURLs for it answers the relay's station URL, so the Wii U connects to the relay as if it were the host.
// Unset: such joins are refused (relay pending).
var RelayStation string

var GRPCFriendsClientConnection *grpc.ClientConn
var GRPCFriendsClient pbfriends.FriendsClient
var GRPCFriendsCommonMetadata metadata.MD
