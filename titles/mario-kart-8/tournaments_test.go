package mk8wiiu

import (
	"database/sql"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	nex "github.com/PretendoNetwork/nex-go/v2"
)

// metaBlob builds the game's chunked metadata blob with a name and description.
func metaBlob(name, desc string) []byte {
	enc := func(s string) []byte {
		var b []byte
		for _, u := range utf16.Encode([]rune(s)) {
			b = binary.BigEndian.AppendUint16(b, u)
		}
		return append(b, 0, 0)
	}
	blob := []byte{0x5a, 0x5a}
	chunk := func(id byte, data []byte) {
		blob = append(blob, id)
		blob = binary.BigEndian.AppendUint16(blob, uint16(len(data)))
		blob = append(blob, data...)
	}
	chunk(2, enc(name))
	chunk(4, enc(desc))
	return append(blob, 255)
}

var codeSeq atomic.Int64

func goodTournament() simpleSearchObject {
	attrs := []uint32{1, 0, 0, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0}
	return simpleSearchObject{
		Attributes: attrs, Metadata: metaBlob("Cup", "desc"), CommunityID: 77,
		CommunityCode: strings.Repeat("0", 12)[:12-len(itoa(codeSeq.Add(1)))] + itoa(codeSeq.Load()),
		DateTime:      dateTimeAttribute{StartDateTime: 100, EndDateTime: 200},
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestVerifyTournament(t *testing.T) {
	if e := verifyTournament(goodTournament()); e != nil {
		t.Fatalf("good tournament refused: %v", e)
	}
	cases := map[string]func(*simpleSearchObject){
		"attribute 0 out of set": func(o *simpleSearchObject) { o.Attributes[0] = 3 },
		"attribute 3 zero":       func(o *simpleSearchObject) { o.Attributes[3] = 0 },
		"attribute 8 too big":    func(o *simpleSearchObject) { o.Attributes[8] = 10 },
		"short attributes":       func(o *simpleSearchObject) { o.Attributes = o.Attributes[:19] },
		"bad metadata":           func(o *simpleSearchObject) { o.Metadata = []byte{1, 2, 3} },
		"huge metadata":          func(o *simpleSearchObject) { o.Metadata = make([]byte, maxTournamentMetadata+1) },
		"empty name":             func(o *simpleSearchObject) { o.Metadata = metaBlob("", "x") },
		"long name":              func(o *simpleSearchObject) { o.Metadata = metaBlob(strings.Repeat("a", 65), "") },
		"long description":       func(o *simpleSearchObject) { o.Metadata = metaBlob("n", strings.Repeat("a", 513)) },
		"control char in name":   func(o *simpleSearchObject) { o.Metadata = metaBlob("a\x07b", "") },
		"starts after it ends":   func(o *simpleSearchObject) { o.DateTime.StartDateTime, o.DateTime.EndDateTime = 300, 200 },
	}
	for name, mut := range cases {
		o := goodTournament()
		mut(&o)
		if verifyTournament(o) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, code := range []string{"", "12345678901", "1234567890123", "12345678901a"} {
		o := goodTournament()
		o.CommunityCode = code
		if verifyCommunity(o) == nil {
			t.Errorf("community code %q accepted", code)
		}
	}
	o := goodTournament()
	o.CommunityID = 0
	if verifyCommunity(o) == nil {
		t.Error("community id 0 accepted")
	}
}

// The store tests need a Postgres; set MK8_TEST_DSN to run them.
func testStore(t *testing.T) *tournamentStore {
	dsn := os.Getenv("MK8_TEST_DSN")
	if dsn == "" {
		t.Skip("MK8_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := newTournamentStore(db)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`DELETE FROM mk8.tournaments`)
	return st
}

func TestTournamentLifecycle(t *testing.T) {
	st := testStore(t)
	const alice, bob = 1800000001, 1800000002
	o := goodTournament()
	id, e := st.create(alice, o)
	if e != nil || id < 20000 {
		t.Fatalf("create: %v id=%d", e, id)
	}
	// duplicate community code
	if _, e := st.create(bob, o); e == nil {
		t.Fatal("duplicate community code accepted")
	}
	// a client claiming another owner is ignored
	o2 := goodTournament()
	o2.Owner = alice
	id2, e := st.create(bob, o2)
	if e != nil {
		t.Fatal(e)
	}
	var owner string
	st.db.QueryRow(`SELECT owner_pid FROM mk8.tournaments WHERE id=$1`, id2).Scan(&owner)
	if owner != "1800000002" {
		t.Fatalf("owner = %s, want the connection's pid", owner)
	}

	// bob cannot modify or delete alice's tournament
	mod := goodTournament()
	mod.Metadata = metaBlob("Renamed", "new")
	if e := st.update(id, bob, mod); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.AccessDenied, "").ResultCode {
		t.Fatalf("non-owner update: %v", e)
	}
	if e := st.delete(id, bob); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.AccessDenied, "").ResultCode {
		t.Fatalf("non-owner delete: %v", e)
	}
	// alice can; the code stays
	mod.CommunityCode = "999999999999"
	if e := st.update(id, alice, mod); e != nil {
		t.Fatal(e)
	}
	var name, ccode string
	st.db.QueryRow(`SELECT name, community_code FROM mk8.tournaments WHERE id=$1`, id).Scan(&name, &ccode)
	if name != "Renamed" || ccode != o.CommunityCode {
		t.Fatalf("after update name=%q code=%q", name, ccode)
	}
	// invalid update leaves it untouched
	bad := goodTournament()
	bad.Attributes[0] = 9
	if e := st.update(id, alice, bad); e == nil {
		t.Fatal("invalid update accepted")
	}
	// missing ids
	if e := st.update(1, alice, mod); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.InvalidIndex, "").ResultCode {
		t.Fatalf("missing update: %v", e)
	}
	if e := st.delete(1, alice); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.InvalidIndex, "").ResultCode {
		t.Fatalf("missing delete: %v", e)
	}
	// scores go with the tournament
	st.db.Exec(`INSERT INTO mk8.tournament_scores (tournament_id, season_id, pid, score, team_id) VALUES ($1,1,5,10,0)`, id)
	if e := st.delete(id, alice); e != nil {
		t.Fatal(e)
	}
	var n int
	st.db.QueryRow(`SELECT count(*) FROM mk8.tournament_scores WHERE tournament_id=$1`, id).Scan(&n)
	if n != 0 {
		t.Fatal("scores survived delete")
	}
	if e := st.delete(id, alice); e == nil {
		t.Fatal("second delete succeeded")
	}
}

func TestTournamentOwnerCap(t *testing.T) {
	st := testStore(t)
	for i := 0; i < maxTournamentsPerOwner; i++ {
		if _, e := st.create(42, goodTournament()); e != nil {
			t.Fatalf("create %d: %v", i, e)
		}
	}
	if _, e := st.create(42, goodTournament()); e == nil {
		t.Fatal("cap not enforced")
	}
	if _, e := st.create(43, goodTournament()); e != nil {
		t.Fatalf("other owner blocked: %v", e)
	}
}
