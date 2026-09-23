package mk8wiiu

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	pb "github.com/PretendoNetwork/grpc/go/account/v2"
	pbfriends "github.com/PretendoNetwork/grpc/go/friends"
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_ranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	common_secure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	common_ticket_granting "github.com/PretendoNetwork/nex-protocols-common-go/v2/ticket-granting"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	mk8ext "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension/mario-kart-8"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	mk8ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/mario-kart-8"
	secure_connection "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"
	ticket_granting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	_ "github.com/lib/pq"
	nexhost "github.com/openpak/nex"
)

// gameServerID is Mario Kart 8's, the id nn-account hands out this server under and the one
// the console's service token names (Kinnay's Wii U game-server list).
const gameServerID = "1010EB00"

// buildName is the string the retail server reported (Kinnay's Wii U game-server list).
const buildName = "branch:origin/project/wup-amk build:3_5_16_2011_0"

type errRequired string

func (e errRequired) Error() string { return string(e) + " is required" }

// Server is one Mario Kart 8 NEX server: an authentication and a secure PRUDP listener, the
// common matchmaking on Postgres, and the title's own tournaments, rankings and DataStore.
type Server struct {
	cfg     nexhost.Config
	db      *sql.DB
	version *nex.LibraryVersion

	authAccount, secureAccount, guestAccount *nex.Account

	friendsClient pbfriends.FriendsClient
	friendsMD     metadata.MD

	matchmaking *common_globals.MatchmakingManager
	rankings    *rankingStore
	tournaments *tournamentStore
}

// Run starts the title and blocks for the life of the process.
func Run(cfg nexhost.Config) error {
	s, err := newServer(cfg)
	if err != nil {
		return err
	}
	go s.serveAuthentication()
	s.serveSecure()
	return errors.New("mario-kart-8: secure server stopped")
}

func newServer(cfg nexhost.Config) (*Server, error) {
	switch {
	case cfg.AccessKey == "":
		return nil, errRequired("NEX_ACCESS_KEY")
	case cfg.DatabaseURL == "":
		return nil, errRequired("NEX_DATABASE_URL")
	case cfg.AccountGRPCAddr == "":
		return nil, errRequired("NEX_ACCOUNT_GRPC_ADDR (nn-account's gRPC)")
	case cfg.PortBase == 0:
		return nil, errRequired("NEX_PORT_BASE")
	}
	version, err := parseVersion(cfg.Version)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("NEX_DATABASE_URL: %w", err)
	}
	s := &Server{cfg: cfg, db: db, version: version}
	if s.rankings, err = newRankingStore(db); err != nil {
		return nil, err
	}
	if s.tournaments, err = newTournamentStore(db); err != nil {
		return nil, err
	}

	host, port := hostPort(cfg.AccountGRPCAddr)
	common_globals.ConnectToAccountGRPC(host, port, cfg.AccountGRPCKey)
	if cfg.FriendsGRPCAddr != "" {
		conn, err := grpc.NewClient("dns:"+cfg.FriendsGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("friends gRPC: %w", err)
		}
		s.friendsClient = pbfriends.NewFriendsClient(conn)
		s.friendsMD = metadata.Pairs("X-API-Key", cfg.FriendsGRPCKey)
	} else {
		log.Print("[mario-kart-8] NEX_FRIENDS_GRPC_ADDR unset: friend rooms and friend rankings will be empty")
	}

	// The kerberos password is per process: every ticket this server mints is checked against it.
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	password := hex.EncodeToString(secret)
	s.authAccount = nex.NewAccount(types.NewPID(1), "Quazal Authentication", password, false)
	s.secureAccount = nex.NewAccount(types.NewPID(2), "Quazal Rendez-Vous", password, false)
	// The guest the retail server knew (Pretendo's server kept it); harmless to keep.
	s.guestAccount = nex.NewAccount(types.NewPID(100), "guest", "MMQea3n!fsik", false)
	return s, nil
}

func parseVersion(v string) (*nex.LibraryVersion, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("NEX_VERSION %q: want major.minor.patch", v)
	}
	n := make([]int, 3)
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("NEX_VERSION %q: %w", v, err)
		}
		n[i] = x
	}
	return nex.NewLibraryVersion(n[0], n[1], n[2]), nil
}

func hostPort(addr string) (string, uint16) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, 0
	}
	p, _ := strconv.Atoi(addr[i+1:])
	return addr[:i], uint16(p)
}

func (s *Server) newPRUDPServer(secure bool) (*nex.PRUDPServer, *nex.PRUDPEndPoint) {
	server := nex.NewPRUDPServer()
	server.ByteStreamSettings.UseStructureHeader = true // NEX >= 3.5 carries structure headers
	server.LibraryVersions.SetDefault(s.version)
	server.AccessKey = s.cfg.AccessKey
	endpoint := nex.NewPRUDPEndPoint(1)
	endpoint.IsSecureEndPoint = secure
	endpoint.ServerAccount = s.authAccount
	if secure {
		endpoint.ServerAccount = s.secureAccount
	}
	endpoint.AccountDetailsByPID = s.accountByPID
	endpoint.AccountDetailsByUsername = s.accountByUsername
	endpoint.OnError(func(err *nex.Error) { log.Printf("[mario-kart-8] %v", err) })
	server.BindPRUDPEndPoint(endpoint)
	if secure {
		go nexhost.WatchBans("mario-kart-8", endpoint)
	}
	return server, endpoint
}

func (s *Server) serveAuthentication() {
	server, endpoint := s.newPRUDPServer(false)

	protocol := ticket_granting.NewProtocol()
	endpoint.RegisterServiceProtocol(protocol)
	common := common_ticket_granting.NewCommonProtocol(protocol)
	station := types.NewStationURL("")
	station.SetURLType(constants.StationURLPRUDP)
	station.SetAddress(s.cfg.SecureHost)
	station.SetPortNumber(uint16(s.cfg.SecurePort()))
	station.SetConnectionID(1)
	station.SetPrincipalID(types.NewPID(2))
	station.SetStreamID(1)
	station.SetStreamType(constants.StreamTypeRVSecure)
	station.SetType(uint8(constants.StationURLFlagPublic))
	common.SecureStationURL = station
	common.BuildName = types.NewString(buildName)
	common.SecureServerAccount = s.secureAccount
	common.ConfigurePNValidation([]string{gameServerID})

	log.Printf("[mario-kart-8] authentication on udp :%d, secure station %s:%d", s.cfg.AuthPort(), s.cfg.SecureHost, s.cfg.SecurePort())
	server.Listen(s.cfg.AuthPort())
}

func (s *Server) serveSecure() {
	server, endpoint := s.newPRUDPServer(true)

	secure := secure_connection.NewProtocol()
	endpoint.RegisterServiceProtocol(secure)
	commonSecure := common_secure.NewCommonProtocol(secure)
	commonSecure.EnableInsecureRegister()
	commonSecure.CreateReportDBRecord = func(types.PID, types.UInt32, types.QBuffer) error { return nil }

	nat := nat_traversal.NewProtocol()
	endpoint.RegisterServiceProtocol(nat)
	common_nat_traversal.NewCommonProtocol(nat)

	s.matchmaking = common_globals.NewMatchmakingManager(endpoint, s.db)
	s.matchmaking.GetUserFriendPIDs = s.friendPIDs

	mm := match_making.NewProtocol()
	endpoint.RegisterServiceProtocol(mm)
	common_match_making.NewCommonProtocol(mm).SetManager(s.matchmaking)

	mmExt := match_making_ext.NewProtocol()
	endpoint.RegisterServiceProtocol(mmExt)
	common_match_making_ext.NewCommonProtocol(mmExt).SetManager(s.matchmaking)

	// Mario Kart 8's MatchmakeExtension adds the tournament methods (36-41); the standard ones
	// stay with the common implementation. The wrapper below answers the title's methods from
	// wire layouts recorded in NintendoClients rather than the library's placeholder types.
	extension := &mk8Extension{Protocol: mk8ext.NewProtocol(endpoint), s: s}
	endpoint.RegisterServiceProtocol(extension)
	common_matchmake_extension.NewCommonProtocol(extension.Protocol).SetManager(s.matchmaking)

	ranking := &mk8Ranking{Protocol: mk8ranking.NewProtocol(endpoint), s: s}
	endpoint.RegisterServiceProtocol(ranking)
	s.rankings.wire(common_ranking.NewCommonProtocol(ranking.Protocol), s.friendPIDs)

	if err := registerDataStore(endpoint, s.db, s.friendPIDs); err != nil {
		log.Fatalf("[mario-kart-8] datastore: %v", err)
	}

	log.Printf("[mario-kart-8] secure on udp :%d (nex %s)", s.cfg.SecurePort(), s.cfg.Version)
	server.Listen(s.cfg.SecurePort())
}

// friendPIDs is the friend graph nn-friends holds for a PID, or nothing when it is unreachable.
func (s *Server) friendPIDs(pid uint32) []uint32 {
	if s.friendsClient == nil {
		return nil
	}
	ctx := metadata.NewOutgoingContext(context.Background(), s.friendsMD)
	resp, err := s.friendsClient.GetUserFriendPIDs(ctx, &pbfriends.GetUserFriendPIDsRequest{Pid: pid})
	if err != nil {
		log.Printf("[mario-kart-8] friends of %d: %v", pid, err)
		return nil
	}
	return resp.Pids
}

func (s *Server) accountByPID(pid types.PID) (*nex.Account, *nex.Error) {
	switch {
	case pid.Equals(s.authAccount.PID):
		return s.authAccount, nil
	case pid.Equals(s.secureAccount.PID):
		return s.secureAccount, nil
	case pid.Equals(s.guestAccount.PID):
		return s.guestAccount, nil
	}
	password, nexErr := s.nexPassword(pid)
	if nexErr != nil {
		return nil, nexErr
	}
	return nex.NewAccount(pid, strconv.FormatUint(uint64(pid), 10), password, false), nil
}

func (s *Server) accountByUsername(username string) (*nex.Account, *nex.Error) {
	switch username {
	case s.authAccount.Username:
		return s.authAccount, nil
	case s.secureAccount.Username:
		return s.secureAccount, nil
	case s.guestAccount.Username:
		return s.guestAccount, nil
	}
	n, err := strconv.ParseUint(username, 10, 64)
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidUsername, "username is not a PID")
	}
	return s.accountByPID(types.NewPID(n))
}

// nexPassword is the console's NEX password as nn-account knows it (GetNEXPassword).
func (s *Server) nexPassword(pid types.PID) (string, *nex.Error) {
	ctx := metadata.NewOutgoingContext(context.Background(), common_globals.GRPCAccountCommonMetadata)
	resp, err := common_globals.GRPCAccountClient.GetNEXPassword(ctx, &pb.GetNEXPasswordRequest{Pid: uint32(pid)})
	if err != nil {
		log.Printf("[mario-kart-8] nex password for %d: %v", uint64(pid), err)
		return "", nex.NewError(nexhost.NEXPasswordResult(err), "no such PID")
	}
	return resp.Password, nil
}
