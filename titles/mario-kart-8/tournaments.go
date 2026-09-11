package mk8wiiu

import (
	"database/sql"
	"fmt"
	"strings"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	mm_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	mk8ext "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension/mario-kart-8"
	"github.com/lib/pq"
)

// mk8Extension answers Mario Kart 8's own MatchmakeExtension methods (tournaments) and leaves
// every standard method to the common implementation attached to the embedded protocol.
type mk8Extension struct {
	*mk8ext.Protocol
	s *Server
}

func (p *mk8Extension) HandlePacket(packet nex.PacketInterface) {
	m := packet.RMCMessage()
	if m.IsRequest && m.ProtocolID == mk8ext.ProtocolID {
		switch m.MethodID {
		case mk8ext.MethodCreateSimpleSearchObject:
			dispatch(packet, mk8ext.ProtocolID, p.s.createTournament)
			return
		case mk8ext.MethodUpdateSimpleSearchObject:
			dispatch(packet, mk8ext.ProtocolID, p.s.updateTournament)
			return
		case mk8ext.MethodDeleteSimpleSearchObject:
			dispatch(packet, mk8ext.ProtocolID, p.s.deleteTournament)
			return
		case mk8ext.MethodSearchSimpleSearchObject:
			dispatch(packet, mk8ext.ProtocolID, p.s.searchTournaments)
			return
		case mk8ext.MethodJoinMatchmakeSessionWithExtraParticipants:
			dispatch(packet, mk8ext.ProtocolID, p.s.joinWithExtraParticipants)
			return
		case mk8ext.MethodSearchSimpleSearchObjectByObjectIDs:
			dispatch(packet, mk8ext.ProtocolID, p.s.tournamentsByID)
			return
		}
	}
	p.Protocol.HandlePacket(packet)
}

// tournamentStore keeps tournaments (the game's "communities") and their scores in Postgres.
// Pretendo's server kept running totals in Redis; here they are sums over the scores table.
type tournamentStore struct{ db *sql.DB }

const tournamentSchema = `
CREATE SCHEMA IF NOT EXISTS mk8;
CREATE SEQUENCE IF NOT EXISTS mk8.tournament_id START 20000;
CREATE TABLE IF NOT EXISTS mk8.tournaments (
	id bigint PRIMARY KEY DEFAULT nextval('mk8.tournament_id'),
	owner_pid numeric(20) NOT NULL,
	attributes bigint[] NOT NULL,
	metadata bytea NOT NULL DEFAULT '',
	community_id bigint NOT NULL,
	community_code text NOT NULL UNIQUE,
	start_daytime bigint NOT NULL DEFAULT 0, end_daytime bigint NOT NULL DEFAULT 0,
	start_time bigint NOT NULL DEFAULT 0, end_time bigint NOT NULL DEFAULT 0,
	start_datetime numeric(20) NOT NULL DEFAULT 0, end_datetime numeric(20) NOT NULL DEFAULT 0,
	total_participants bigint NOT NULL DEFAULT 0,
	season_id bigint NOT NULL DEFAULT 1,
	name text NOT NULL DEFAULT '', description text NOT NULL DEFAULT '',
	created timestamptz NOT NULL DEFAULT now(), updated timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS mk8.tournament_scores (
	tournament_id bigint NOT NULL REFERENCES mk8.tournaments(id) ON DELETE CASCADE,
	season_id bigint NOT NULL,
	pid numeric(20) NOT NULL,
	score bigint NOT NULL,
	team_id int NOT NULL,
	team_score bigint NOT NULL DEFAULT 0,
	metadata bytea NOT NULL DEFAULT '',
	last_update timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (tournament_id, season_id, pid)
);
`

func newTournamentStore(db *sql.DB) (*tournamentStore, error) {
	if _, err := db.Exec(tournamentSchema); err != nil {
		return nil, fmt.Errorf("tournament schema: %w", err)
	}
	return &tournamentStore{db: db}, nil
}

// verify is the game's own validity envelope for a tournament (Pretendo's server checked the
// same ranges): 20 attributes, the enumerated ones within their sets, parseable metadata.
func verifyTournament(o simpleSearchObject) *nex.Error {
	a := o.Attributes
	if len(a) != 20 {
		return invalid("tournament needs 20 attributes, got %d", len(a))
	}
	in := func(v uint32, allowed ...uint32) bool {
		for _, x := range allowed {
			if v == x {
				return true
			}
		}
		return false
	}
	switch {
	case !in(a[0], 1, 2), a[2] > 5, a[3] == 0 || a[3] > 8, !in(a[4], 1, 2), !in(a[5], 1, 2, 3),
		!in(a[6], 1, 2), !in(a[7], 1, 2), a[8] == 0 || a[8] > 9, a[9] > 4, !in(a[10], 1, 2),
		!in(a[11], 1, 2, 3, 4), !in(a[12], 1, 2), !in(a[13], 1, 2):
		return invalid("tournament attribute out of range")
	}
	if _, err := parseTournamentMetadata(o.Metadata); err != nil {
		return invalid("%v", err)
	}
	return nil
}

const tournamentColumns = `id, owner_pid, attributes, metadata, community_id, community_code,
	start_daytime, end_daytime, start_time, end_time, start_datetime, end_datetime`

func scanTournament(row interface{ Scan(...any) error }) (simpleSearchObject, error) {
	var o simpleSearchObject
	var attrs pq.Int64Array
	var id, cid int64
	var owner, sdt, edt string
	var sd, ed, st, et int64
	if err := row.Scan(&id, &owner, &attrs, &o.Metadata, &cid, &o.CommunityCode, &sd, &ed, &st, &et, &sdt, &edt); err != nil {
		return o, err
	}
	o.ID, o.CommunityID = uint32(id), uint32(cid)
	fmt.Sscan(owner, &o.Owner)
	fmt.Sscan(sdt, &o.DateTime.StartDateTime)
	fmt.Sscan(edt, &o.DateTime.EndDateTime)
	o.DateTime.StartDaytime, o.DateTime.EndDaytime = uint32(sd), uint32(ed)
	o.DateTime.StartTime, o.DateTime.EndTime = uint32(st), uint32(et)
	o.Attributes = make([]uint32, len(attrs))
	for i, v := range attrs {
		o.Attributes[i] = uint32(v)
	}
	o.LivelinessUpdateTime = uint64(types.DateTime(0).Now())
	return o, nil
}

func int64s(v []uint32) pq.Int64Array {
	out := make(pq.Int64Array, len(v))
	for i, x := range v {
		out[i] = int64(x)
	}
	return out
}

func (s *Server) createTournament(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	o, err := readSimpleSearchObject(in)
	if err != nil {
		return invalid("%v", err)
	}
	if nexErr := verifyTournament(o); nexErr != nil {
		return nexErr
	}
	if len(o.CommunityCode) != 12 || o.CommunityID == 0 || strings.Trim(o.CommunityCode, "0123456789") != "" {
		return invalid("community code must be 12 digits")
	}
	meta, _ := parseTournamentMetadata(o.Metadata)
	var id int64
	err = s.db.QueryRow(`INSERT INTO mk8.tournaments (owner_pid, attributes, metadata, community_id, community_code,
			start_daytime, end_daytime, start_time, end_time, start_datetime, end_datetime, name, description)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		uint64(conn.PID()), int64s(o.Attributes), o.Metadata, int64(o.CommunityID), o.CommunityCode,
		int64(o.DateTime.StartDaytime), int64(o.DateTime.EndDaytime), int64(o.DateTime.StartTime), int64(o.DateTime.EndTime),
		fmt.Sprint(o.DateTime.StartDateTime), fmt.Sprint(o.DateTime.EndDateTime), meta.Name, meta.Description).Scan(&id)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return invalid("community code already used")
		}
		return storeError(err)
	}
	out.WriteUInt32LE(uint32(id))
	return nil
}

func (s *Server) updateTournament(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	id, err := readU32(in)
	if err != nil {
		return invalid("%v", err)
	}
	o, err := readSimpleSearchObject(in)
	if err != nil {
		return invalid("%v", err)
	}
	if nexErr := verifyTournament(o); nexErr != nil {
		return nexErr
	}
	if nexErr := s.tournamentOwnedBy(id, conn.PID()); nexErr != nil {
		return nexErr
	}
	meta, _ := parseTournamentMetadata(o.Metadata)
	_, err = s.db.Exec(`UPDATE mk8.tournaments SET attributes=$2, metadata=$3, start_daytime=$4, end_daytime=$5,
			start_time=$6, end_time=$7, start_datetime=$8, end_datetime=$9, name=$10, description=$11, updated=now() WHERE id=$1`,
		int64(id), int64s(o.Attributes), o.Metadata, int64(o.DateTime.StartDaytime), int64(o.DateTime.EndDaytime),
		int64(o.DateTime.StartTime), int64(o.DateTime.EndTime), fmt.Sprint(o.DateTime.StartDateTime), fmt.Sprint(o.DateTime.EndDateTime),
		meta.Name, meta.Description)
	return storeError(err)
}

func (s *Server) deleteTournament(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	id, err := readU32(in)
	if err != nil {
		return invalid("%v", err)
	}
	if nexErr := s.tournamentOwnedBy(id, conn.PID()); nexErr != nil {
		return nexErr
	}
	_, err = s.db.Exec(`DELETE FROM mk8.tournaments WHERE id=$1`, int64(id))
	return storeError(err)
}

func (s *Server) tournamentOwnedBy(id uint32, pid types.PID) *nex.Error {
	var owner string
	err := s.db.QueryRow(`SELECT owner_pid FROM mk8.tournaments WHERE id=$1`, int64(id)).Scan(&owner)
	if err == sql.ErrNoRows {
		return nex.NewError(nex.ResultCodes.Core.InvalidIndex, "no such tournament")
	}
	if err != nil {
		return storeError(err)
	}
	if owner != fmt.Sprint(uint64(pid)) {
		return nex.NewError(nex.ResultCodes.Core.AccessDenied, "not the tournament's owner")
	}
	return nil
}

func (s *Server) searchTournaments(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	p, err := readSimpleSearchParam(in)
	if err != nil {
		return invalid("%v", err)
	}
	if len(p.CommunityCode) > 12 || p.Range.Length > 100 {
		return invalid("search param out of range")
	}
	where, args := []string{"true"}, []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if p.ID != 0 {
		add("id = $%d", int64(p.ID))
	}
	if p.Owner != 0 {
		add("owner_pid = $%d", fmt.Sprint(p.Owner))
	}
	if p.CommunityCode != "" {
		add("community_code = $%d", p.CommunityCode)
	}
	ops := map[uint32]string{1: "=", 2: ">", 3: "<", 4: ">=", 5: "<="}
	for i, c := range p.Conditions {
		if c.Operator == 0 {
			continue
		}
		op, ok := ops[c.Operator]
		if !ok {
			return invalid("unknown search operator %d", c.Operator)
		}
		args = append(args, int64(c.Value))
		where = append(where, fmt.Sprintf("attributes[%d] %s $%d", i+1, op, len(args)))
	}
	limit := p.Range.Length
	if limit == 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT `+tournamentColumns+` FROM mk8.tournaments WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY id OFFSET %d LIMIT %d`, p.Range.Offset, limit), args...)
	if err != nil {
		return storeError(err)
	}
	defer rows.Close()
	var result []simpleSearchObject
	for rows.Next() {
		o, err := scanTournament(rows)
		if err != nil {
			return storeError(err)
		}
		result = append(result, o)
	}
	writeSimpleSearchObjects(out, result)
	return nil
}

func (s *Server) tournamentsByID(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	ids, err := readU32List(in)
	if err != nil {
		return invalid("%v", err)
	}
	if len(ids) > 100 {
		return invalid("too many ids")
	}
	rows, err := s.db.Query(`SELECT `+tournamentColumns+` FROM mk8.tournaments WHERE id = ANY($1) ORDER BY id`, int64s(ids))
	if err != nil {
		return storeError(err)
	}
	defer rows.Close()
	var result []simpleSearchObject
	for rows.Next() {
		o, err := scanTournament(rows)
		if err != nil {
			return storeError(err)
		}
		result = append(result, o)
	}
	writeSimpleSearchObjects(out, result)
	return nil
}

// joinWithExtraParticipants is JoinMatchmakeSession with a participant count: the same join
// the common protocol does, with vacantParticipants set from the request.
func (s *Server) joinWithExtraParticipants(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	gid, err := readU32(in)
	if err != nil {
		return invalid("%v", err)
	}
	joinMessage, err := readString(in)
	if err != nil {
		return invalid("%v", err)
	}
	if _, err := readBool(in); err != nil { // ignoreBlacklist
		return invalid("%v", err)
	}
	participationCount, err := in.ReadUInt16LE()
	if err != nil {
		return invalid("%v", err)
	}
	if _, err := readU32(in); err != nil { // extraParticipants
		return invalid("%v", err)
	}
	if len(joinMessage) > 256 {
		return invalid("join message too long")
	}
	endpoint := conn.Endpoint().(*nex.PRUDPEndPoint)
	s.matchmaking.Mutex.Lock()
	defer s.matchmaking.Mutex.Unlock()
	session, _, nexErr := mm_database.GetMatchmakeSessionByID(s.matchmaking, endpoint, gid)
	if nexErr != nil {
		return nexErr
	}
	if nexErr := common_globals.CanJoinMatchmakeSession(s.matchmaking, conn.PID(), session); nexErr != nil {
		return nexErr
	}
	if participationCount == 0 {
		participationCount = 1
	}
	if _, nexErr := mm_database.JoinMatchmakeSession(s.matchmaking, session, conn, participationCount, joinMessage); nexErr != nil {
		return nexErr
	}
	session.SessionKey.WriteTo(out)
	return nil
}
