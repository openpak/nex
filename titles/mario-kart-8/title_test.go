package mk8wiiu

import (
	"encoding/binary"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
)

func streams() (*nex.ByteStreamOut, func([]byte) *nex.ByteStreamIn) {
	lib := nex.NewLibraryVersions()
	lib.SetDefault(nex.NewLibraryVersion(3, 5, 4))
	settings := nex.NewByteStreamSettings()
	settings.UseStructureHeader = true
	return nex.NewByteStreamOut(lib, settings), func(b []byte) *nex.ByteStreamIn { return nex.NewByteStreamIn(b, lib, settings) }
}

// The wire shapes are hand-written from NintendoClients' layouts, so a write must read back
// identically, structure headers included.
func TestSimpleSearchObjectRoundTrip(t *testing.T) {
	out, in := streams()
	want := simpleSearchObject{ID: 20001, Owner: 1800000005, Attributes: make([]uint32, 20), Metadata: []byte{0x5a, 0x5a, 0xff},
		CommunityID: 7, CommunityCode: "123456789012", DateTime: dateTimeAttribute{1, 2, 3, 4, 5, 6}, LivelinessRate: 9, LivelinessUpdateTime: 11}
	want.Attributes[0] = 1
	want.write(out)
	got, err := readSimpleSearchObject(in(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Owner != want.Owner || got.CommunityCode != want.CommunityCode || got.DateTime != want.DateTime ||
		got.LivelinessRate != 9 || got.LivelinessUpdateTime != 11 || len(got.Attributes) != 20 || got.Attributes[0] != 1 {
		t.Fatalf("round trip changed the object: %+v", got)
	}
}

func TestTournamentMetadata(t *testing.T) {
	name := []byte{0, 'O', 0, 'K', 0, 0}
	blob := []byte{0x5a, 0x5a, 2}
	blob = binary.BigEndian.AppendUint16(blob, uint16(len(name)))
	blob = append(blob, name...)
	blob = append(blob, 5, 0, 4, 0, 0, 0, 3, 255)
	m, err := parseTournamentMetadata(blob)
	if err != nil || m.Name != "OK" || m.RepeatType != 3 {
		t.Fatalf("got %+v, %v", m, err)
	}
	if _, err := parseTournamentMetadata([]byte{1, 2, 3}); err == nil {
		t.Fatal("bad magic accepted")
	}
	if verifyTournament(simpleSearchObject{Attributes: make([]uint32, 19)}) == nil {
		t.Fatal("19 attributes accepted")
	}
}

func TestCompetitionInfoWrites(t *testing.T) {
	out, in := streams()
	competitionScoreInfo{SeasonID: 3, Scores: []competitionScore{{Rank: 1, PID: 5, Score: 100, TeamID: 1, Metadata: []byte{1}}}, NumParticipants: 1, TeamScores: []uint32{1, 2, 3, 4}}.write(out)
	s := in(out.Bytes())
	var hdr [5]byte
	if b, err := s.Read(5); err != nil || copy(hdr[:], b) != 5 || hdr[0] != 0 {
		t.Fatalf("structure header: %v %v", b, err)
	}
	if v, _ := s.ReadUInt32LE(); v != 3 {
		t.Fatalf("season = %d", v)
	}
	if n, _ := s.ReadUInt32LE(); n != 1 {
		t.Fatalf("scores = %d", n)
	}
}
