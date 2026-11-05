package histgrm

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	boss "github.com/PretendoNetwork/grpc/go/boss/v2"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TaskID is the BOSS task the game polls.
const TaskID = "histgrm"

// App is one Pikmin 3 BOSS application.
type App struct {
	ID      string
	TitleID uint64
	Region  string
}

// Apps are the BOSS app ids nn-boss lists for Pikmin 3 (list-known-boss-apps.ts), the trial included.
var Apps = []App{
	{"9jRZEoWYLc3OG9a8", 0x000500001012BD00, "USA"},
	{"4krJA4Gx3jF5nhQf", 0x000500001012BE00, "EUR"},
	{"VWqUTspR5YtjDjxa", 0x000500001012BC00, "JPN"},
	{"XcawL2u1CU624gg3", 0x0005000010185300, "JPN trial"},
}

func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

// Generate reads the rankings table and builds the file.
func Generate(db *sql.DB) ([]byte, int, error) {
	rows, err := db.Query(`SELECT category, score FROM rankings ORDER BY category`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	by := map[uint32][]uint32{}
	for rows.Next() {
		var c, s int32
		if err := rows.Scan(&c, &s); err != nil {
			return nil, 0, err
		}
		by[uint32(c)] = append(by[uint32(c)], uint32(s))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	cats := make([]Category, 0, len(by))
	for id, scores := range by {
		cats = append(cats, Distribute(id, scores))
	}
	return Build(cats), len(cats), nil
}

// Run is the scheduled job. Flags (any order): -register registers the tasks and stops,
// -dry-run generates and reports without uploading.
//
// Environment: PN_PIKMIN3_POSTGRES_URI (NEX_DATABASE_URL), HISTGRM_GRPC_HOST (openpak-nn-boss:20081),
// HISTGRM_GRPC_APIKEY (PN_BOSS_CONFIG_GRPC_BOSS_SERVER_API_KEY), HISTGRM_APP_IDS, HISTGRM_FILE_NAME,
// HISTGRM_FILE_TYPE, HISTGRM_STATE_DIR (/data), HISTGRM_ACK_UNVERIFIED_FORMAT.
func Run(args []string) error {
	var registerOnly, dry bool
	for _, a := range args {
		switch a {
		case "-register", "--register":
			registerOnly = true
		case "-dry-run", "--dry-run":
			dry = true
		default:
			return fmt.Errorf("unknown flag %q", a)
		}
	}
	apps := Apps
	if ids := env("HISTGRM_APP_IDS", ""); ids != "" {
		apps = nil
		for _, id := range strings.Split(ids, ",") {
			apps = append(apps, App{ID: strings.TrimSpace(id)})
		}
	}

	key := env("HISTGRM_GRPC_APIKEY", os.Getenv("PN_BOSS_CONFIG_GRPC_BOSS_SERVER_API_KEY"))
	if key == "" {
		return errors.New("missing HISTGRM_GRPC_APIKEY")
	}
	conn, err := grpc.NewClient(env("HISTGRM_GRPC_HOST", "openpak-nn-boss:20081"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := boss.NewBossServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "X-API-Key", key)

	// Tasks first: nn-boss serves a 404 tasksheet until the task exists, and it needs no BOSS keys.
	for _, a := range apps {
		_, err := client.RegisterTask(ctx, &boss.RegisterTaskRequest{
			Id: TaskID, BossAppId: a.ID, TitleId: a.TitleID, Status: "open", Description: "Pikmin 3 ranking histogram " + a.Region,
		})
		switch {
		case err == nil:
			log.Printf("registered task %s for %s (%s)", TaskID, a.ID, a.Region)
		case alreadyRegistered(err):
		default:
			return fmt.Errorf("register %s: %w", a.ID, err)
		}
	}
	if registerOnly {
		return nil
	}

	dsn := env("PN_PIKMIN3_POSTGRES_URI", os.Getenv("NEX_DATABASE_URL"))
	if dsn == "" {
		return errors.New("missing PN_PIKMIN3_POSTGRES_URI / NEX_DATABASE_URL")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	data, n, err := Generate(db)
	if err != nil {
		return err
	}
	if n == 0 {
		log.Print("no scores yet, nothing to publish")
		return nil
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	log.Printf("generated %d bytes for %d categories (sha256 %s)", len(data), n, hash[:12])

	stateDir := env("HISTGRM_STATE_DIR", "/data")
	last := stateDir + "/last.sha256"
	if prev, err := os.ReadFile(last); err == nil && strings.TrimSpace(string(prev)) == hash {
		log.Print("unchanged since the last upload")
		return nil
	}
	if dry {
		log.Print("dry run: not uploading")
		return nil
	}
	if env("HISTGRM_ACK_UNVERIFIED_FORMAT", "") != "1" {
		return errors.New("refusing to upload: the histgrm layout is unverified against the game (see docs/histgrm.md); set HISTGRM_ACK_UNVERIFIED_FORMAT=1 to override")
	}

	for _, a := range apps {
		res, err := client.UploadFileWUP(ctx, &boss.UploadFileWUPRequest{
			TaskId: TaskID, BossAppId: a.ID, Name: env("HISTGRM_FILE_NAME", "histgrm"), Type: env("HISTGRM_FILE_TYPE", "AppData"),
			Attributes: &boss.FileAttributes{}, NotifyOnNew: []string{"app"}, Data: data,
		})
		if err != nil {
			return fmt.Errorf("upload %s: %w", a.ID, err)
		}
		log.Printf("%s: uploaded, data ID %d", a.ID, res.GetFile().GetDataId())
	}
	if err := os.MkdirAll(stateDir, 0o755); err == nil {
		_ = os.WriteFile(last, []byte(hash+"\n"), 0o644)
	}
	return nil
}

// alreadyRegistered reports whether a RegisterTask error only says the task exists. nn-boss answers
// that with InvalidArgument and "ALREADY_EXISTS: ..." in the message, not with codes.AlreadyExists.
func alreadyRegistered(err error) bool {
	if status.Code(err) == codes.AlreadyExists {
		return true
	}
	return status.Code(err) == codes.InvalidArgument &&
		strings.Contains(strings.ToLower(status.Convert(err).Message()), "already_exists")
}
