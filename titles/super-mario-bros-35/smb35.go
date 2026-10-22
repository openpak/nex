// Package smb35 is the Super Mario Bros. 35 title server: the NEX protocols the game
// speaks, registered on OpenPak's shared NEX host (openpak/nex).
//
// The title is a preservation-only target. Nintendo delisted Super Mario Bros. 35 and
// switched its service off on 31 March 2021, so there is no live service to compete with
// and no vendor left to validate a token against. Nobody plays this game online at all
// unless someone runs a server like this one. docs/design.md says what that changes.
//
// Everything here is either a Pretendo common protocol wired to a manager, or one of the
// two protocols Pretendo has no common implementation for: Ranking2 and MatchmakeReferee.
// Nothing in this package reimplements PRUDP, kerberos, matchmaking or messaging.
package smb35

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_message_delivery "github.com/PretendoNetwork/nex-protocols-common-go/v2/message-delivery"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	matchmake_referee "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-referee"
	message_delivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	ranking2 "github.com/PretendoNetwork/nex-protocols-go/v2/ranking2"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	nexhost "github.com/openpak/nex"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
)

// Config is what this title needs on top of the shared host's own configuration
// (nexhost.FromEnv reads that half).
type Config struct {
	DatabaseURL string // Postgres, for the Pretendo common matchmaking/messaging/utility managers
	EagleURL    string // base URL of the Eagle relay; the gathering ID is appended as a path segment
	EagleKey    []byte // HMAC-SHA256 key the relay verifies handoff tokens with
}

// FromEnv reads the title's own configuration.
func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("SMB35_DATABASE_URL"),
		EagleURL:    strings.TrimSuffix(os.Getenv("SMB35_EAGLE_URL"), "/"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("SMB35_DATABASE_URL is required: matchmaking, messaging and the unique-id table are backed by Postgres")
	}
	if key := os.Getenv("SMB35_EAGLE_KEY"); key != "" {
		raw, err := hex.DecodeString(strings.TrimSpace(key))
		if err != nil || len(raw) < 16 {
			return cfg, errors.New("SMB35_EAGLE_KEY must be at least 16 bytes of hex")
		}
		cfg.EagleKey = raw
	}
	// A relay address without a key would hand every player a token the relay must refuse,
	// which is worse than no handoff at all: the game would open a socket and be turned away.
	if cfg.EagleURL != "" && cfg.EagleKey == nil {
		return cfg, errors.New("SMB35_EAGLE_URL is set but SMB35_EAGLE_KEY is not: handoff tokens would not verify")
	}
	return cfg, nil
}

// Server is the title's own state: a database handle for the Pretendo managers, and the
// two in-memory stores for the protocols Pretendo has no manager for.
type Server struct {
	host        *nexhost.Host
	cfg         Config
	db          *sql.DB
	matchmaking *common_globals.MatchmakingManager
	common      *ranking2Store
	rounds      *roundStore
}

// New opens the database and registers every protocol Super Mario Bros. 35 calls on the
// host's secure endpoint. The caller still has to Run the host.
func New(host *nexhost.Host, cfg Config) (*Server, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// The common protocols create their own schemas and only log if they cannot, so an
	// unreachable database would otherwise surface as a server that starts cleanly and then
	// fails every single call. Find out here instead.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("reach database: %w", err)
	}

	s := &Server{
		host:   host,
		cfg:    cfg,
		db:     db,
		common: newRanking2Store(),
		rounds: newRoundStore(),
	}

	s.registerMatchmaking()
	s.registerMessaging()
	s.registerUtility()
	s.registerRanking2()
	s.registerMatchmakeReferee()

	if cfg.EagleURL == "" {
		log.Print("[smb35] no SMB35_EAGLE_URL: lobbies will fill and no match will ever start, because the game waits for a relay address that never arrives")
	}
	return s, nil
}

// Close releases the database handle.
func (s *Server) Close() error { return s.db.Close() }

// registerMatchmaking wires the three matchmaking protocols to one manager.
//
// Super Mario Bros. 35's matchmaking is the plain NEX shape and needs no title logic: the
// game calls AutoMatchmakeWithParamPostpone with a MatchmakeSession whose maxParticipants
// is 35, the server finds a session matching the search criteria or creates the proposed
// one, joins the caller and notifies the participants already there. The 35 is the client's
// number, never ours, and the server never counts to it or decides a lobby is ready —
// nothing in NEX starts the match. What happens when the lobby fills happens on the Eagle
// relay and on the clients; see eagle.go.
func (s *Server) registerMatchmaking() {
	manager := common_globals.NewMatchmakingManager(s.host.Secure, s.db)
	manager.GetUserFriendPIDs = s.host.FriendPIDs
	s.matchmaking = manager

	// Order matters: match-making's SetManager creates the "matchmaking" schema the other
	// two put their tables in.
	mm := match_making.NewProtocol()
	s.host.Secure.RegisterServiceProtocol(mm)
	common_match_making.NewCommonProtocol(mm).SetManager(manager)

	ext := matchmake_extension.NewProtocol()
	s.host.Secure.RegisterServiceProtocol(ext)
	commonExt := common_matchmake_extension.NewCommonProtocol(ext)
	commonExt.SetManager(manager)
	commonExt.OnAfterAutoMatchmakeWithParamPostpone = s.afterAutoMatchmake
	commonExt.OnAfterJoinMatchmakeSessionWithParam = s.afterJoinWithParam
	commonExt.OnAfterJoinMatchmakeSession = s.afterJoin

	mmExt := match_making_ext.NewProtocol()
	s.host.Secure.RegisterServiceProtocol(mmExt)
	common_match_making_ext.NewCommonProtocol(mmExt).SetManager(manager)
}

// registerMessaging wires MessageDelivery, the in-lobby relay for short text and binary
// messages. A recipient is either a PID or a gathering, and the gathering case resolves its
// participants through the matchmaking manager, so that has to exist first.
func (s *Server) registerMessaging() {
	manager := common_globals.NewMessagingManager(s.host.Secure, s.db)
	manager.MatchmakingManager = s.matchmaking

	protocol := message_delivery.NewProtocol()
	s.host.Secure.RegisterServiceProtocol(protocol)
	common_message_delivery.NewCommonProtocol(protocol).SetManager(manager)
}

// registerUtility wires the NEX unique-id methods and answers GetIntegerSettings with the
// game's own tuning table.
func (s *Server) registerUtility() {
	manager := common_globals.NewUtilityManager(s.host.Secure, s.db)
	manager.GetIntegerSettings = integerSettings

	protocol := utility.NewProtocol()
	s.host.Secure.RegisterServiceProtocol(protocol)
	common_utility.NewCommonProtocol(protocol).SetManager(manager)
}

func (s *Server) registerRanking2() {
	protocol := ranking2.NewProtocol()
	protocol.GetCommonData = s.getCommonData
	protocol.PutCommonData = s.putCommonData
	protocol.GetRanking = s.getRanking
	protocol.GetCategorySetting = s.getCategorySetting
	protocol.GetEstimateScoreRank = s.getEstimateScoreRank
	s.host.Secure.RegisterServiceProtocol(protocol)
}

func (s *Server) registerMatchmakeReferee() {
	protocol := matchmake_referee.NewProtocol()
	protocol.StartRound = s.startRound
	protocol.GetStartRoundParam = s.getStartRoundParam
	protocol.EndRound = s.endRound
	protocol.EndRoundWithoutReport = s.endRoundWithoutReport
	s.host.Secure.RegisterServiceProtocol(protocol)
}

// integerSettings is Utility::GetIntegerSettings. The game reads round and battle tuning
// out of index 0; index 10 is a second table nobody has identified a single value of, and
// answering it empty is what earlier research notes record. Any other index is an error, which
// is also recorded there: an unknown index is a client bug, not a missing feature.
func integerSettings(_ *common_globals.UtilityManager, _ types.PID, index types.UInt32) (types.Map[types.UInt16, types.UInt32], *nex.Error) {
	switch index {
	case 0:
		return integerSettings0(), nil
	case 10:
		return types.NewMap[types.UInt16, types.UInt32](), nil
	default:
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, fmt.Sprintf("no integer settings at index %d", index))
	}
}

// integerSettings0 is the table the game asks for at index 0.
//
// The indices have no names on the wire and nobody has recovered what any one of them
// means, so these are carried across exactly as earlier research notes had them rather than
// reasoned about. Changing a value is guesswork until someone can watch a match react.
func integerSettings0() types.Map[types.UInt16, types.UInt32] {
	return types.Map[types.UInt16, types.UInt32]{
		0: 60, 1: 30, 2: 90, 3: 1, 4: 0, 5: 0, 6: 0, 7: 0, 8: 0,
		9: 0, 10: 5, 11: 3, 12: 1, 13: 30, 14: 30, 15: 180, 16: 0,
	}
}

// afterAutoMatchmake, afterJoinWithParam and afterJoin all mean one thing to this title: a
// player is now in a lobby and needs the relay's address. The common protocol runs these
// after it has answered the client and notified the other participants, so the handoff is
// all that is left to do.
func (s *Server) afterAutoMatchmake(packet nex.PacketInterface, _ match_making_types.AutoMatchmakeParam) {
	s.sendEagleHandoff(packet)
}

func (s *Server) afterJoinWithParam(packet nex.PacketInterface, _ match_making_types.JoinMatchmakeSessionParam) {
	s.sendEagleHandoff(packet)
}

func (s *Server) afterJoin(packet nex.PacketInterface, _ types.UInt32, _ types.String) {
	s.sendEagleHandoff(packet)
}

// Register wires the title's protocols onto the shared host.
func Register(host *nexhost.Host) error {
	cfg, err := FromEnv()
	if err != nil {
		return err
	}
	_, err = New(host, cfg) // lives as long as the process; nothing closes it
	return err
}

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "super-mario-bros-35", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
}
