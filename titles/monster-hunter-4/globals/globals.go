package globals

import (
	"database/sql"

	pbfriends "github.com/PretendoNetwork/grpc/go/friends"
	"github.com/PretendoNetwork/nex-go/v2"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/plogger-go"
	"github.com/openpak/nex/internal/coremessaging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var Postgres *sql.DB
var MatchmakingManager *common_globals.MatchmakingManager
var MessagingManager *common_globals.MessagingManager

// CoreMessaging is set when NEX_CORE_STORAGE names "messaging": stored messages go to chat.
var CoreMessaging *coremessaging.Store
var Logger *plogger.Logger
var KerberosPassword = "password" // * Default password

var AuthenticationServer *nex.PRUDPServer
var AuthenticationEndpoint *nex.PRUDPEndPoint

var SecureServer *nex.PRUDPServer
var SecureEndpoint *nex.PRUDPEndPoint

var GRPCFriendsClientConnection *grpc.ClientConn
var GRPCFriendsClient pbfriends.FriendsClient
var GRPCFriendsCommonMetadata metadata.MD
