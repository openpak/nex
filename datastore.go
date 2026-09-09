package mps

// DataStore (0x73), for real rather than stubbed.
//
// nex-protocols-common-go implements every DataStore method the game can call, but it holds
// no data of its own: it asks the operator for object metadata through a set of callbacks
// and for object bytes through an S3 presigner. This file is those callbacks — a Postgres
// table of object metadata and a MinIO/S3 client — so get_meta, get_metas, search_object,
// change_meta, rate_object, delete_object and the prepare/complete upload and download
// handshake all answer with real state that survives a restart.
//
// The metadata row keeps the whole DataStoreMetaInfo as one serialised blob and mirrors only
// the fields something queries by into columns. That keeps writes to one statement and means
// a new field in a future structure revision needs no migration.
//
// ponytail: one table, no object cache, no soft-delete retention policy. If DataStore ever
// carries enough Mario Party Superstars traffic for that to matter, the columns to index are
// already there.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	datastore_constants "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// noPersistenceSlot is the persistence slot ID meaning "this object is not persistent". An
// object posted with any other slot replaces whatever the owner last put in that slot.
const noPersistenceSlot = 0xFFFF

type store struct {
	db       *sql.DB
	endpoint *nex.PRUDPEndPoint
	friends  func(uint32) []uint32
}

func newStore(db *sql.DB, endpoint *nex.PRUDPEndPoint) *store {
	return &store{db: db, endpoint: endpoint}
}

func (s *store) migrate() error {
	_, err := s.db.Exec(`
		CREATE SCHEMA IF NOT EXISTS datastore;
		CREATE TABLE IF NOT EXISTS datastore.objects (
			data_id bigserial PRIMARY KEY,
			owner_pid numeric(20) NOT NULL,
			data_type integer NOT NULL DEFAULT 0,
			name text NOT NULL DEFAULT '',
			size bigint NOT NULL DEFAULT 0,
			refer_data_id bigint NOT NULL DEFAULT 0,
			tags text[] NOT NULL DEFAULT '{}',
			persistence_slot integer,
			access_password numeric(20) NOT NULL DEFAULT 0,
			update_password numeric(20) NOT NULL DEFAULT 0,
			upload_completed boolean NOT NULL DEFAULT false,
			deleted boolean NOT NULL DEFAULT false,
			created_time timestamp NOT NULL DEFAULT now(),
			updated_time timestamp NOT NULL DEFAULT now(),
			meta bytea NOT NULL
		);
		CREATE INDEX IF NOT EXISTS objects_owner ON datastore.objects (owner_pid) WHERE NOT deleted;
		CREATE UNIQUE INDEX IF NOT EXISTS objects_persistence
			ON datastore.objects (owner_pid, persistence_slot)
			WHERE persistence_slot IS NOT NULL AND NOT deleted;
	`)
	return err
}

// wire hands the common protocol every callback it looks for. A callback left nil there
// makes its whole method answer Core::NotImplemented, which is the stub this replaces.
func (s *store) wire(common *common_datastore.CommonProtocol) {
	s.friends = common.GetUserFriendPIDs

	common.GetObjectInfoByDataID = s.objectByDataID
	common.GetObjectInfoByDataIDWithPassword = s.objectByDataIDWithPassword
	common.GetObjectInfoByPersistenceTargetWithPassword = s.objectByPersistenceTarget
	common.GetObjectOwnerByDataID = s.ownerByDataID
	common.GetObjectSizeByDataID = s.sizeByDataID
	common.GetObjectInfosByDataStoreSearchParam = s.search

	common.InitializeObjectByPreparePostParam = s.initializeObject
	common.InitializeObjectRatingWithSlot = s.initializeRating
	common.UpdateObjectUploadCompletedByDataID = s.setUploadCompleted

	// The library's field names for these two are swapped against what it passes them:
	// the "period" setter is called with the new period, the "data type" setter with the
	// new data type, but their parameter names say the opposite. Named here by what they
	// actually carry.
	common.UpdateObjectPeriodByDataIDWithPassword = s.updatePeriod
	common.UpdateObjectDataTypeByDataIDWithPassword = s.updateDataType
	common.UpdateObjectMetaBinaryByDataIDWithPassword = s.updateMetaBinary

	common.RateObjectWithPassword = s.rate
	common.DeleteObjectByDataID = func(dataID types.UInt64) *nex.Error { return s.delete(uint64(dataID)) }
	common.DeleteObjectByDataIDWithPassword = s.deleteWithPassword
}

// --- reads -------------------------------------------------------------------------------

// objectByDataID returns an object only once its upload has completed. The library relies on
// that: complete_post_object treats "this returns something" as "already uploaded".
func (s *store) objectByDataID(dataID types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.selectMeta(`data_id = $1 AND upload_completed`, uint64(dataID))
}

func (s *store) objectByDataIDWithPassword(dataID types.UInt64, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	meta, nexErr := s.objectByDataID(dataID)
	if nexErr != nil {
		return meta, nexErr
	}
	return meta, s.checkPassword(uint64(dataID), "access_password", uint64(password))
}

func (s *store) objectByPersistenceTarget(target datastore_types.DataStorePersistenceTarget, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	if uint16(target.PersistenceSlotID) == noPersistenceSlot {
		return datastore_types.NewDataStoreMetaInfo(), nex.NewError(nex.ResultCodes.DataStore.NotFound, "no persistence slot given")
	}
	meta, nexErr := s.selectMeta(`owner_pid = $1 AND persistence_slot = $2 AND upload_completed`,
		uint64(target.OwnerID), uint16(target.PersistenceSlotID))
	if nexErr != nil {
		return meta, nexErr
	}
	return meta, s.checkPassword(uint64(meta.DataID), "access_password", uint64(password))
}

func (s *store) ownerByDataID(dataID types.UInt64) (uint32, *nex.Error) {
	var owner uint64
	err := s.db.QueryRow(`SELECT owner_pid FROM datastore.objects WHERE data_id = $1 AND NOT deleted`, uint64(dataID)).Scan(&owner)
	if err != nil {
		return 0, notFoundOr(err)
	}
	return uint32(owner), nil
}

func (s *store) sizeByDataID(dataID types.UInt64) (uint32, *nex.Error) {
	var size uint64
	err := s.db.QueryRow(`SELECT size FROM datastore.objects WHERE data_id = $1 AND NOT deleted`, uint64(dataID)).Scan(&size)
	if err != nil {
		return 0, notFoundOr(err)
	}
	return uint32(size), nil
}

func (s *store) selectMeta(where string, args ...any) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	var blob []byte
	query := `SELECT meta FROM datastore.objects WHERE NOT deleted AND ` + where
	if err := s.db.QueryRow(query, args...).Scan(&blob); err != nil {
		return datastore_types.NewDataStoreMetaInfo(), notFoundOr(err)
	}
	return s.decode(blob)
}

// checkPassword answers InvalidPassword for a wrong one. Password 0 means the caller offered
// none, which is fine for an object that has none set.
func (s *store) checkPassword(dataID uint64, column string, offered uint64) *nex.Error {
	var expected uint64
	err := s.db.QueryRow(`SELECT `+column+` FROM datastore.objects WHERE data_id = $1`, dataID).Scan(&expected)
	if err != nil {
		return notFoundOr(err)
	}
	if expected != 0 && expected != offered {
		return nex.NewError(nex.ResultCodes.DataStore.InvalidPassword, fmt.Sprintf("wrong %s for object %d", column, dataID))
	}
	return nil
}

// --- writes ------------------------------------------------------------------------------

// initializeObject reserves a data ID for an upload that has not happened yet. The object
// stays invisible to reads until complete_post_object marks the upload finished.
func (s *store) initializeObject(ownerPID types.PID, param datastore_types.DataStorePreparePostParam) (uint64, *nex.Error) {
	slot := uint16(param.PersistenceInitParam.PersistenceSlotID)
	persistent := slot != noPersistenceSlot

	transaction, err := s.db.Begin()
	if err != nil {
		return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	defer transaction.Rollback() //nolint:errcheck // rolled back only if Commit did not run

	if persistent {
		// One object per owner and slot: the previous occupant either goes entirely or
		// just loses the slot, which is what DeleteLastObject decides.
		statement := `UPDATE datastore.objects SET persistence_slot = NULL WHERE owner_pid = $1 AND persistence_slot = $2`
		if param.PersistenceInitParam.DeleteLastObject {
			statement = `UPDATE datastore.objects SET persistence_slot = NULL, deleted = true WHERE owner_pid = $1 AND persistence_slot = $2`
		}
		if _, err := transaction.Exec(statement, uint64(ownerPID), slot); err != nil {
			return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
		}
	}

	var dataID uint64
	err = transaction.QueryRow(`
		INSERT INTO datastore.objects
			(owner_pid, data_type, name, size, refer_data_id, tags, persistence_slot, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, ''::bytea)
		RETURNING data_id`,
		uint64(ownerPID), uint16(param.DataType), string(param.Name), uint32(param.Size),
		uint32(param.ReferDataID), pq.Array(stringList(param.Tags)), nullableSlot(persistent, slot),
	).Scan(&dataID)
	if err != nil {
		return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	meta := metaFromPreparePost(dataID, ownerPID, param)
	if _, err := transaction.Exec(`UPDATE datastore.objects SET meta = $2 WHERE data_id = $1`, dataID, s.encode(meta)); err != nil {
		return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	if err := transaction.Commit(); err != nil {
		return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return dataID, nil
}

// initializeRating adds one rating slot to an object that was just prepared.
func (s *store) initializeRating(dataID uint64, param datastore_types.DataStoreRatingInitParamWithSlot) *nex.Error {
	return s.mutate(dataID, func(meta *datastore_types.DataStoreMetaInfo) {
		rating := datastore_types.NewDataStoreRatingInfoWithSlot()
		rating.Slot = param.Slot
		rating.Rating.InitialValue = param.Param.InitialValue
		rating.Rating.TotalValue = param.Param.InitialValue
		rating.Rating.Count = types.NewUInt32(0)

		meta.Ratings = append(meta.Ratings, rating)
	})
}

func (s *store) setUploadCompleted(dataID types.UInt64, completed bool) *nex.Error {
	result, err := s.db.Exec(`UPDATE datastore.objects SET upload_completed = $2, updated_time = now() WHERE data_id = $1 AND NOT deleted`, uint64(dataID), completed)
	if err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return affectedOrNotFound(result, uint64(dataID))
}

func (s *store) updatePeriod(dataID types.UInt64, period types.UInt16, password types.UInt64) *nex.Error {
	if nexErr := s.checkPassword(uint64(dataID), "update_password", uint64(password)); nexErr != nil {
		return nexErr
	}
	return s.mutate(uint64(dataID), func(meta *datastore_types.DataStoreMetaInfo) { meta.Period = period })
}

func (s *store) updateDataType(dataID types.UInt64, dataType types.UInt16, password types.UInt64) *nex.Error {
	if nexErr := s.checkPassword(uint64(dataID), "update_password", uint64(password)); nexErr != nil {
		return nexErr
	}
	if _, err := s.db.Exec(`UPDATE datastore.objects SET data_type = $2 WHERE data_id = $1`, uint64(dataID), uint16(dataType)); err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return s.mutate(uint64(dataID), func(meta *datastore_types.DataStoreMetaInfo) { meta.DataType = dataType })
}

func (s *store) updateMetaBinary(dataID types.UInt64, metaBinary types.QBuffer, password types.UInt64) *nex.Error {
	if nexErr := s.checkPassword(uint64(dataID), "update_password", uint64(password)); nexErr != nil {
		return nexErr
	}
	return s.mutate(uint64(dataID), func(meta *datastore_types.DataStoreMetaInfo) { meta.MetaBinary = metaBinary })
}

// rate adds one vote to a rating slot and answers the slot's new total.
func (s *store) rate(dataID types.UInt64, slot types.UInt8, value types.Int32, password types.UInt64) (datastore_types.DataStoreRatingInfo, *nex.Error) {
	if nexErr := s.checkPassword(uint64(dataID), "access_password", uint64(password)); nexErr != nil {
		return datastore_types.NewDataStoreRatingInfo(), nexErr
	}

	rated := datastore_types.NewDataStoreRatingInfo()
	found := false
	nexErr := s.mutate(uint64(dataID), func(meta *datastore_types.DataStoreMetaInfo) {
		for i := range meta.Ratings {
			if meta.Ratings[i].Slot != types.Int8(slot) {
				continue
			}
			meta.Ratings[i].Rating.TotalValue += types.Int64(value)
			meta.Ratings[i].Rating.Count++
			rated = meta.Ratings[i].Rating
			found = true
		}
	})
	if nexErr != nil {
		return rated, nexErr
	}
	if !found {
		return rated, nex.NewError(nex.ResultCodes.DataStore.NotFound, fmt.Sprintf("object %d has no rating slot %d", dataID, slot))
	}
	return rated, nil
}

func (s *store) delete(dataID uint64) *nex.Error {
	result, err := s.db.Exec(`UPDATE datastore.objects SET deleted = true, persistence_slot = NULL WHERE data_id = $1 AND NOT deleted`, dataID)
	if err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return affectedOrNotFound(result, dataID)
}

func (s *store) deleteWithPassword(dataID types.UInt64, password types.UInt64) *nex.Error {
	if nexErr := s.checkPassword(uint64(dataID), "update_password", uint64(password)); nexErr != nil {
		return nexErr
	}
	return s.delete(uint64(dataID))
}

// mutate reads an object's metadata, lets fn change it, and writes it back together with the
// columns derived from it. One statement each way, inside a transaction, so a rating vote
// racing another one cannot lose a count.
func (s *store) mutate(dataID uint64, fn func(*datastore_types.DataStoreMetaInfo)) *nex.Error {
	transaction, err := s.db.Begin()
	if err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	defer transaction.Rollback() //nolint:errcheck // rolled back only if Commit did not run

	var blob []byte
	if err := transaction.QueryRow(`SELECT meta FROM datastore.objects WHERE data_id = $1 AND NOT deleted FOR UPDATE`, dataID).Scan(&blob); err != nil {
		return notFoundOr(err)
	}

	meta, nexErr := s.decode(blob)
	if nexErr != nil {
		return nexErr
	}

	fn(&meta)
	meta.UpdatedTime = meta.UpdatedTime.FromTimestamp(time.Now().UTC())

	if _, err := transaction.Exec(`UPDATE datastore.objects SET meta = $2, name = $3, tags = $4, updated_time = now() WHERE data_id = $1`,
		dataID, s.encode(meta), string(meta.Name), pq.Array(stringList(meta.Tags))); err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	if err := transaction.Commit(); err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return nil
}

// --- search ------------------------------------------------------------------------------

// search answers SearchObject. The caller's permission to see each result is checked by the
// common protocol afterwards, so this filters on what the query says and not on who is
// asking — except for the owner-type narrowing, which decides which rows are candidates at
// all.
//
// ponytail: SearchTarget's permission-shaped variants (SearchTypePublic, SearchTypeSend…)
// are not translated into SQL. They narrow by the same permission field the common protocol
// already enforces on every row it returns, so ignoring them costs a wider scan, never a
// leak. Translate them if a search ever has to page through more rows than this title has.
func (s *store) search(param datastore_types.DataStoreSearchParam, pid types.PID) ([]datastore_types.DataStoreMetaInfo, uint32, *nex.Error) {
	where := []string{"NOT deleted", "upload_completed"}
	var args []any

	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	switch param.OwnerType {
	case datastore_constants.SearchTargetFriend:
		add("owner_pid = ANY($%d)", pq.Array(pidsToInt64(s.friendsOf(uint32(pid)))))
	case datastore_constants.SearchTargetAnybodyExcludeSpecified:
		if len(param.OwnerIDs) > 0 {
			add("NOT (owner_pid = ANY($%d))", pq.Array(pidListToInt64(param.OwnerIDs)))
		}
	default: // SearchTargetAnybody
		if len(param.OwnerIDs) > 0 {
			add("owner_pid = ANY($%d)", pq.Array(pidListToInt64(param.OwnerIDs)))
		}
	}

	switch {
	case len(param.DataTypes) > 0:
		add("data_type = ANY($%d)", pq.Array(uint16ListToInt64(param.DataTypes)))
	case param.DataType != 0:
		add("data_type = $%d", uint16(param.DataType))
	}

	if param.ReferDataID != 0 {
		add("refer_data_id = $%d", uint32(param.ReferDataID))
	}
	if len(param.Tags) > 0 {
		add("tags @> $%d", pq.Array(stringList(param.Tags)))
	}
	if timestamp, ok := dateTime(param.CreatedAfter); ok {
		add("created_time >= $%d", timestamp)
	}
	if timestamp, ok := dateTime(param.CreatedBefore); ok {
		add("created_time <= $%d", timestamp)
	}
	if timestamp, ok := dateTime(param.UpdatedAfter); ok {
		add("updated_time >= $%d", timestamp)
	}
	if timestamp, ok := dateTime(param.UpdatedBefore); ok {
		add("updated_time <= $%d", timestamp)
	}

	condition := strings.Join(where, " AND ")

	var total uint32
	if err := s.db.QueryRow(`SELECT count(*) FROM datastore.objects WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	offset, length := uint32(param.ResultRange.Offset), uint32(param.ResultRange.Length)
	if length == 0 {
		length = 100 // the client asked for nothing; a page is friendlier than everything
	}

	query := fmt.Sprintf(`SELECT meta FROM datastore.objects WHERE %s ORDER BY %s LIMIT %d OFFSET %d`,
		condition, orderBy(param), length, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	defer rows.Close()

	var objects []datastore_types.DataStoreMetaInfo
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
		}
		meta, nexErr := s.decode(blob)
		if nexErr != nil {
			return nil, 0, nexErr
		}
		objects = append(objects, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	return objects, total, nil
}

func (s *store) friendsOf(pid uint32) []uint32 {
	if s.friends == nil {
		return nil
	}
	return s.friends(pid)
}

// orderBy maps the search's sort column onto a column this table actually has. Anything it
// does not (the rating slots) falls back to data ID, which is at least a stable order.
func orderBy(param datastore_types.DataStoreSearchParam) string {
	column := "data_id"
	switch param.ResultOrderColumn {
	case datastore_constants.SearchSortColumnSize:
		column = "size"
	case datastore_constants.SearchSortColumnNameAlphabetical:
		column = "name"
	case datastore_constants.SearchSortColumnDataType:
		column = "data_type"
	case datastore_constants.SearchSortColumnCreatedTime:
		column = "created_time"
	case datastore_constants.SearchSortColumnUpdatedTime:
		column = "updated_time"
	}
	if param.ResultOrder != datastore_constants.SearchSortOrderAsc {
		return column + " DESC"
	}
	return column + " ASC"
}

// --- encoding ----------------------------------------------------------------------------

func (s *store) encode(meta datastore_types.DataStoreMetaInfo) []byte {
	stream := nex.NewByteStreamOut(s.endpoint.LibraryVersions(), s.endpoint.ByteStreamSettings())
	meta.WriteTo(stream)
	return stream.Bytes()
}

func (s *store) decode(blob []byte) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	meta := datastore_types.NewDataStoreMetaInfo()
	stream := nex.NewByteStreamIn(blob, s.endpoint.LibraryVersions(), s.endpoint.ByteStreamSettings())
	if err := meta.ExtractFrom(stream); err != nil {
		return meta, nex.NewError(nex.ResultCodes.DataStore.SystemFileError, err.Error())
	}
	return meta, nil
}

func metaFromPreparePost(dataID uint64, ownerPID types.PID, param datastore_types.DataStorePreparePostParam) datastore_types.DataStoreMetaInfo {
	now := time.Now().UTC()

	meta := datastore_types.NewDataStoreMetaInfo()
	meta.DataID = types.NewUInt64(dataID)
	meta.OwnerID = ownerPID
	meta.Size = param.Size
	meta.Name = param.Name
	meta.DataType = param.DataType
	meta.MetaBinary = param.MetaBinary
	meta.Permission = param.Permission
	meta.DelPermission = param.DelPermission
	meta.CreatedTime = meta.CreatedTime.FromTimestamp(now)
	meta.UpdatedTime = meta.UpdatedTime.FromTimestamp(now)
	meta.Period = param.Period
	meta.Status = datastore_constants.DataStatusNone
	meta.ReferredCnt = types.NewUInt32(0)
	meta.ReferDataID = param.ReferDataID
	meta.Flag = param.Flag
	meta.ReferredTime = meta.ReferredTime.FromTimestamp(now)
	// Period is a lifetime in days; 0 means the object does not expire.
	meta.ExpireTime = meta.ExpireTime.FromTimestamp(now.AddDate(0, 0, int(param.Period)))
	meta.Tags = param.Tags
	meta.Ratings = types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()
	return meta
}

// --- object storage ----------------------------------------------------------------------

// S3Config is the object store DataStore hands out presigned URLs for. Without it the
// metadata half of DataStore still works and the upload and download methods answer
// Core::NotImplemented, which is exactly what they did before this file existed.
type S3Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

func s3FromEnv() S3Config {
	return S3Config{
		Endpoint:  os.Getenv("MPS_S3_ENDPOINT"),
		Bucket:    env("MPS_S3_BUCKET", "mario-party-superstars"),
		AccessKey: os.Getenv("MPS_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("MPS_S3_SECRET_KEY"),
		UseSSL:    os.Getenv("MPS_S3_USE_SSL") == "1",
	}
}

func (c S3Config) attach(common *common_datastore.CommonProtocol) error {
	if c.Endpoint == "" {
		return errors.New("MPS_S3_ENDPOINT is not set")
	}

	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
	})
	if err != nil {
		return err
	}

	common.SetMinIOClient(client)
	common.S3Bucket = c.Bucket
	common.SetDataKeyBase("objects")
	common.SetNotifyKeyBase("notifications")
	return nil
}

// --- small helpers -----------------------------------------------------------------------

func notFoundOr(err error) *nex.Error {
	if errors.Is(err, sql.ErrNoRows) {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
}

func affectedOrNotFound(result sql.Result, dataID uint64) *nex.Error {
	affected, err := result.RowsAffected()
	if err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	if affected == 0 {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, fmt.Sprintf("no such object %d", dataID))
	}
	return nil
}

func nullableSlot(persistent bool, slot uint16) any {
	if !persistent {
		return nil
	}
	return slot
}

// dateTime reports whether a search bound was actually set. A zero DateTime is "unbounded",
// not "the year zero", and turning it into a real timestamp would exclude every row.
func dateTime(value types.DateTime) (time.Time, bool) {
	if value == 0 {
		return time.Time{}, false
	}
	return value.Standard(), true
}

func stringList(list types.List[types.String]) []string {
	out := make([]string, len(list))
	for i, value := range list {
		out[i] = string(value)
	}
	return out
}

func pidListToInt64(list types.List[types.PID]) []int64 {
	out := make([]int64, len(list))
	for i, pid := range list {
		out[i] = int64(pid)
	}
	return out
}

func pidsToInt64(pids []uint32) []int64 {
	out := make([]int64, len(pids))
	for i, pid := range pids {
		out[i] = int64(pid)
	}
	return out
}

func uint16ListToInt64(list types.List[types.UInt16]) []int64 {
	out := make([]int64, len(list))
	for i, value := range list {
		out[i] = int64(value)
	}
	return out
}
