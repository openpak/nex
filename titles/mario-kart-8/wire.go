package mk8wiiu

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"unicode/utf16"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/globals"
)

// The Mario Kart 8 structures below follow the layouts NintendoClients records for the title
// (nintendo/nex/matchmaking_mk8d.py, ranking_mk8d.py), which are taken from captures. The Go
// protocol library's types for them are placeholders, so they are read and written here.

// rawHandler answers one RMC method from its parameters; what it writes to out is the response.
type rawHandler func(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error

// dispatch runs a rawHandler for the packet and sends its result or its error.
func dispatch(packet nex.PacketInterface, protocolID uint16, h rawHandler) {
	req := packet.RMCMessage()
	conn := packet.Sender().(*nex.PRUDPConnection)
	endpoint := conn.Endpoint().(*nex.PRUDPEndPoint)
	in := nex.NewByteStreamIn(req.Parameters, endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	if nexErr := h(in, conn, out); nexErr != nil {
		globals.RespondError(packet, protocolID, nexErr)
		return
	}
	resp := nex.NewRMCSuccess(endpoint, out.Bytes())
	resp.ProtocolID = protocolID
	resp.MethodID = req.MethodID
	resp.CallID = req.CallID
	globals.Respond(packet, resp)
}

func invalid(format string, a ...any) *nex.Error {
	return nex.NewError(nex.ResultCodes.Core.InvalidArgument, fmt.Sprintf(format, a...))
}

// --- primitive helpers ---

func readU32(in *nex.ByteStreamIn) (uint32, error) { return in.ReadUInt32LE() }
func readU8(in *nex.ByteStreamIn) (uint8, error)   { return in.ReadUInt8() }
func readBool(in *nex.ByteStreamIn) (bool, error)  { return in.ReadBool() }
func readString(in *nex.ByteStreamIn) (string, error) {
	var s types.String
	err := s.ExtractFrom(in)
	return string(s), err
}
func readQBuffer(in *nex.ByteStreamIn) ([]byte, error) {
	var b types.QBuffer
	err := b.ExtractFrom(in)
	return []byte(b), err
}
func readPID(in *nex.ByteStreamIn) (uint64, error) {
	var p types.PID
	err := p.ExtractFrom(in)
	return uint64(p), err
}
func readDateTime(in *nex.ByteStreamIn) (uint64, error) {
	var d types.DateTime
	err := d.ExtractFrom(in)
	return uint64(d), err
}
func readU32List(in *nex.ByteStreamIn) ([]uint32, error) {
	var l types.List[types.UInt32]
	if err := l.ExtractFrom(in); err != nil {
		return nil, err
	}
	out := make([]uint32, len(l))
	for i, v := range l {
		out[i] = uint32(v)
	}
	return out, nil
}

// readStructure reads a structure header (when the stream carries them) and hands the body
// reader to fn; the header's length is trusted only for being present.
func readStructure(in *nex.ByteStreamIn, fn func(in *nex.ByteStreamIn) error) error {
	var st types.Structure
	if err := st.ExtractHeaderFrom(in); err != nil {
		return err
	}
	return fn(in)
}

// writeStructure writes fn's output as one structure with the header the stream expects.
func writeStructure(out *nex.ByteStreamOut, fn func(body *nex.ByteStreamOut)) {
	body := out.CopyNew().(*nex.ByteStreamOut)
	fn(body)
	content := body.Bytes()
	types.Structure{}.WriteHeaderTo(out, uint32(len(content)))
	out.Write(content)
}

func writeString(out *nex.ByteStreamOut, s string)   { types.NewString(s).WriteTo(out) }
func writeQBuffer(out *nex.ByteStreamOut, b []byte)  { types.NewQBuffer(b).WriteTo(out) }
func writePID(out *nex.ByteStreamOut, p uint64)      { types.NewPID(p).WriteTo(out) }
func writeDateTime(out *nex.ByteStreamOut, d uint64) { types.NewDateTime(d).WriteTo(out) }

// fromTime is a NEX DateTime for a Go time.
func fromTime(t time.Time) types.DateTime {
	var d types.DateTime
	return d.FromTimestamp(t)
}
func writeU32List(out *nex.ByteStreamOut, v []uint32) {
	out.WriteUInt32LE(uint32(len(v)))
	for _, x := range v {
		out.WriteUInt32LE(x)
	}
}

// --- ResultRange ---

type resultRange struct{ Offset, Length uint32 }

func readResultRange(in *nex.ByteStreamIn) (resultRange, error) {
	var r types.ResultRange
	err := r.ExtractFrom(in)
	return resultRange{uint32(r.Offset), uint32(r.Length)}, err
}

// --- SimpleSearchObject (a tournament) ---

type dateTimeAttribute struct {
	StartDaytime, EndDaytime, StartTime, EndTime uint32
	StartDateTime, EndDateTime                   uint64
}

type simpleSearchObject struct {
	ID                   uint32
	Owner                uint64
	Attributes           []uint32
	Metadata             []byte
	CommunityID          uint32
	CommunityCode        string
	DateTime             dateTimeAttribute
	LivelinessRate       uint32
	LivelinessUpdateTime uint64
}

func readDateTimeAttribute(in *nex.ByteStreamIn) (a dateTimeAttribute, err error) {
	err = readStructure(in, func(in *nex.ByteStreamIn) error {
		var e error
		if a.StartDaytime, e = readU32(in); e != nil {
			return e
		}
		if a.EndDaytime, e = readU32(in); e != nil {
			return e
		}
		if a.StartTime, e = readU32(in); e != nil {
			return e
		}
		if a.EndTime, e = readU32(in); e != nil {
			return e
		}
		if a.StartDateTime, e = readDateTime(in); e != nil {
			return e
		}
		a.EndDateTime, e = readDateTime(in)
		return e
	})
	return
}

func (a dateTimeAttribute) write(out *nex.ByteStreamOut) {
	writeStructure(out, func(b *nex.ByteStreamOut) {
		b.WriteUInt32LE(a.StartDaytime)
		b.WriteUInt32LE(a.EndDaytime)
		b.WriteUInt32LE(a.StartTime)
		b.WriteUInt32LE(a.EndTime)
		writeDateTime(b, a.StartDateTime)
		writeDateTime(b, a.EndDateTime)
	})
}

func readSimpleSearchObject(in *nex.ByteStreamIn) (o simpleSearchObject, err error) {
	err = readStructure(in, func(in *nex.ByteStreamIn) error {
		var e error
		if o.ID, e = readU32(in); e != nil {
			return e
		}
		if o.Owner, e = readPID(in); e != nil {
			return e
		}
		if o.Attributes, e = readU32List(in); e != nil {
			return e
		}
		if o.Metadata, e = readQBuffer(in); e != nil {
			return e
		}
		if o.CommunityID, e = readU32(in); e != nil {
			return e
		}
		if o.CommunityCode, e = readString(in); e != nil {
			return e
		}
		if o.DateTime, e = readDateTimeAttribute(in); e != nil {
			return e
		}
		if o.LivelinessRate, e = readU32(in); e != nil {
			return e
		}
		o.LivelinessUpdateTime, e = readDateTime(in)
		return e
	})
	return
}

func (o simpleSearchObject) write(out *nex.ByteStreamOut) {
	writeStructure(out, func(b *nex.ByteStreamOut) {
		b.WriteUInt32LE(o.ID)
		writePID(b, o.Owner)
		writeU32List(b, o.Attributes)
		writeQBuffer(b, o.Metadata)
		b.WriteUInt32LE(o.CommunityID)
		writeString(b, o.CommunityCode)
		o.DateTime.write(b)
		b.WriteUInt32LE(o.LivelinessRate)
		writeDateTime(b, o.LivelinessUpdateTime)
	})
}

func writeSimpleSearchObjects(out *nex.ByteStreamOut, objects []simpleSearchObject) {
	out.WriteUInt32LE(uint32(len(objects)))
	for _, o := range objects {
		o.write(out)
	}
}

// --- SimpleSearchParam ---

type searchCondition struct{ Value, Operator uint32 }

type simpleSearchParam struct {
	ID            uint32
	Owner         uint64
	Conditions    []searchCondition
	CommunityCode string
	Range         resultRange
	DateTime      uint64
}

func readSimpleSearchParam(in *nex.ByteStreamIn) (p simpleSearchParam, err error) {
	err = readStructure(in, func(in *nex.ByteStreamIn) error {
		var e error
		if p.ID, e = readU32(in); e != nil {
			return e
		}
		if p.Owner, e = readPID(in); e != nil {
			return e
		}
		n, e := readU32(in)
		if e != nil {
			return e
		}
		if n > 64 {
			return errors.New("too many search conditions")
		}
		for i := uint32(0); i < n; i++ {
			var c searchCondition
			if e := readStructure(in, func(in *nex.ByteStreamIn) error {
				var e error
				if c.Value, e = readU32(in); e != nil {
					return e
				}
				c.Operator, e = readU32(in)
				return e
			}); e != nil {
				return e
			}
			p.Conditions = append(p.Conditions, c)
		}
		if p.CommunityCode, e = readString(in); e != nil {
			return e
		}
		if p.Range, e = readResultRange(in); e != nil {
			return e
		}
		p.DateTime, e = readDateTime(in)
		return e
	})
	return
}

// --- competition ranking ---

type competitionScore struct {
	Rank       uint32
	PID        uint64
	Score      uint32
	LastUpdate uint64
	TeamID     uint8
	Metadata   []byte
}

type competitionScoreInfo struct {
	SeasonID        uint32
	Scores          []competitionScore
	NumParticipants uint32
	TeamScores      []uint32
}

func (i competitionScoreInfo) write(out *nex.ByteStreamOut) {
	writeStructure(out, func(b *nex.ByteStreamOut) {
		b.WriteUInt32LE(i.SeasonID)
		b.WriteUInt32LE(uint32(len(i.Scores)))
		for _, sc := range i.Scores {
			writeStructure(b, func(c *nex.ByteStreamOut) {
				c.WriteUInt32LE(sc.Rank)
				writePID(c, sc.PID)
				c.WriteUInt32LE(sc.Score)
				writeDateTime(c, sc.LastUpdate)
				c.WriteUInt8(sc.TeamID)
				writeQBuffer(c, sc.Metadata)
			})
		}
		b.WriteUInt32LE(i.NumParticipants)
		writeU32List(b, i.TeamScores)
	})
}

type competitionInfo struct {
	ID              uint32
	NumParticipants uint32
	TeamScores      []uint32
}

func (i competitionInfo) write(out *nex.ByteStreamOut) {
	writeStructure(out, func(b *nex.ByteStreamOut) {
		b.WriteUInt32LE(i.ID)
		b.WriteUInt32LE(i.NumParticipants)
		writeU32List(b, i.TeamScores)
	})
}

type uploadScoreParam struct {
	ID, SeasonID, Unknown, Score uint32
	TeamID                       uint8
	TeamScore                    uint32
	IsFirstUpload                bool
	Metadata                     []byte
}

func readUploadScoreParam(in *nex.ByteStreamIn) (p uploadScoreParam, err error) {
	err = readStructure(in, func(in *nex.ByteStreamIn) error {
		var e error
		if p.ID, e = readU32(in); e != nil {
			return e
		}
		if p.SeasonID, e = readU32(in); e != nil {
			return e
		}
		if p.Unknown, e = readU32(in); e != nil {
			return e
		}
		if p.Score, e = readU32(in); e != nil {
			return e
		}
		if p.TeamID, e = readU8(in); e != nil {
			return e
		}
		if p.TeamScore, e = readU32(in); e != nil {
			return e
		}
		if p.IsFirstUpload, e = readBool(in); e != nil {
			return e
		}
		p.Metadata, e = readQBuffer(in)
		return e
	})
	return
}

// --- tournament metadata (the game's chunked blob) ---

// tournamentMetadata is what the game packs into SimpleSearchObject.Metadata: big-endian
// chunks (magic 0x5a5a, then id:u8 size:u16 data… until id 255) carrying UTF-16BE strings.
type tournamentMetadata struct {
	Name, Description, RedTeam, BlueTeam                    string
	Revision, IconType                                      uint8
	RepeatType, GamesetNum, BattleTime, UpdateDate, Version uint32
}

func parseTournamentMetadata(data []byte) (tournamentMetadata, error) {
	var m tournamentMetadata
	if len(data) < 3 || binary.BigEndian.Uint16(data) != 0x5a5a {
		return m, errors.New("tournament metadata: bad magic")
	}
	chunks := map[uint8][]byte{}
	p := 2
	for {
		if p >= len(data) {
			return m, errors.New("tournament metadata: unterminated")
		}
		id := data[p]
		p++
		if id == 255 {
			break
		}
		if id > 12 || p+2 > len(data) {
			return m, errors.New("tournament metadata: bad chunk")
		}
		size := int(binary.BigEndian.Uint16(data[p:]))
		p += 2
		if p+size > len(data) {
			return m, errors.New("tournament metadata: chunk overruns")
		}
		chunks[id] = data[p : p+size]
		p += size
	}
	str := func(b []byte) string {
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			v := binary.BigEndian.Uint16(b[i:])
			if v == 0 {
				break
			}
			u = append(u, v)
		}
		return string(utf16.Decode(u))
	}
	u32 := func(b []byte) uint32 {
		if len(b) < 4 {
			return 0
		}
		return binary.BigEndian.Uint32(b)
	}
	if b := chunks[0]; len(b) > 0 {
		m.Revision = b[0]
	}
	m.Version = u32(chunks[1])
	m.Name = str(chunks[2])
	if b := chunks[3]; len(b) > 0 {
		m.IconType = b[0]
	}
	m.Description = str(chunks[4])
	m.RepeatType = u32(chunks[5])
	m.GamesetNum = u32(chunks[6])
	m.RedTeam = str(chunks[7])
	m.BlueTeam = str(chunks[8])
	m.BattleTime = u32(chunks[9])
	m.UpdateDate = u32(chunks[11])
	return m, nil
}
