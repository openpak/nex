package mvdk

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
	common_secure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	common_ticket_granting "github.com/PretendoNetwork/nex-protocols-common-go/v2/ticket-granting"
	secure_connection "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"
	ticket_granting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	_ "github.com/lib/pq"
	nexhost "github.com/openpak/nex"
)

// gameServerIDs are the ids nn-account hands this server out under: the low 32 bits of the
// Wii U and the 3DS title ids, one row per console type in nn-account's servers table.
var gameServerIDs = []string{"10149300", "0012C800"}

type errRequired string

func (e errRequired) Error() string { return string(e) + " is required" }

// Server is one Tipping Stars NEX server: authentication and secure UDP listeners and the
// DataStore on Postgres.
type Server struct {
	cfg     nexhost.Config
	db      *sql.DB
	version *nex.LibraryVersion

	authAccount, secureAccount *nex.Account

	friendsClient pbfriends.FriendsClient
	friendsMD     metadata.MD
}

// Run starts the title and blocks for the life of the process.
func Run(cfg nexhost.Config) error {
	s, err := newServer(cfg)
	if err != nil {
		return err
	}
	go s.serveAuthentication()
	s.serveSecure()
	return errors.New("mario-vs-donkey-kong-tipping-stars: secure server stopped")
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

	host, port := hostPort(cfg.AccountGRPCAddr)
	common_globals.ConnectToAccountGRPC(host, port, cfg.AccountGRPCKey)
	if cfg.FriendsGRPCAddr != "" {
		conn, err := grpc.NewClient("dns:"+cfg.FriendsGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("friends gRPC: %w", err)
		}
		s.friendsClient = pbfriends.NewFriendsClient(conn)
		s.friendsMD = metadata.Pairs("X-API-Key", cfg.FriendsGRPCKey)
	}

	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	password := hex.EncodeToString(secret)
	s.authAccount = nex.NewAccount(types.NewPID(1), "Quazal Authentication", password, false)
	s.secureAccount = nex.NewAccount(types.NewPID(2), "Quazal Rendez-Vous", password, false)
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
	server.ByteStreamSettings.UseStructureHeader = true
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
	endpoint.OnError(func(err *nex.Error) { log.Printf("[mvdk] %v", err) })
	server.BindPRUDPEndPoint(endpoint)
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
	common.SecureServerAccount = s.secureAccount
	common.ConfigurePNValidation(gameServerIDs)
	log.Printf("[mvdk] authentication on udp :%d, secure station %s:%d", s.cfg.AuthPort(), s.cfg.SecureHost, s.cfg.SecurePort())
	server.Listen(s.cfg.AuthPort())
}

func (s *Server) serveSecure() {
	server, endpoint := s.newPRUDPServer(true)
	secure := secure_connection.NewProtocol()
	endpoint.RegisterServiceProtocol(secure)
	commonSecure := common_secure.NewCommonProtocol(secure)
	commonSecure.EnableInsecureRegister()
	commonSecure.CreateReportDBRecord = func(types.PID, types.UInt32, types.QBuffer) error { return nil }
	if err := registerDataStore(endpoint, s.db, s.friendPIDs); err != nil {
		log.Fatalf("[mvdk] datastore: %v", err)
	}
	log.Printf("[mvdk] secure on udp :%d (nex %s)", s.cfg.SecurePort(), s.cfg.Version)
	server.Listen(s.cfg.SecurePort())
}

func (s *Server) friendPIDs(pid uint32) []uint32 {
	if s.friendsClient == nil {
		return nil
	}
	ctx := metadata.NewOutgoingContext(context.Background(), s.friendsMD)
	resp, err := s.friendsClient.GetUserFriendPIDs(ctx, &pbfriends.GetUserFriendPIDsRequest{Pid: pid})
	if err != nil {
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
	}
	ctx := metadata.NewOutgoingContext(context.Background(), common_globals.GRPCAccountCommonMetadata)
	resp, err := common_globals.GRPCAccountClient.GetNEXPassword(ctx, &pb.GetNEXPasswordRequest{Pid: uint32(pid)})
	if err != nil {
		log.Printf("[mvdk] nex password for %d: %v", uint64(pid), err)
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidUsername, "no such PID")
	}
	return nex.NewAccount(pid, strconv.FormatUint(uint64(pid), 10), resp.Password, false), nil
}

func (s *Server) accountByUsername(username string) (*nex.Account, *nex.Error) {
	switch username {
	case s.authAccount.Username:
		return s.authAccount, nil
	case s.secureAccount.Username:
		return s.secureAccount, nil
	}
	n, err := strconv.ParseUint(username, 10, 64)
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidUsername, "username is not a PID")
	}
	return s.accountByPID(types.NewPID(n))
}
