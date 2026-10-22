package smo

// DataStore (0x73) for Super Mario Odyssey, for real.
//
// This is the whole game. Balloon World is nothing but DataStore: PreparePostObject +
// CompletePostObject hide a balloon, SearchObject + PrepareGetObject find and fetch one,
// RateObject/GetRating are the "clap" votes, DeleteObject retires a balloon. The retail
// server backs the object bodies (the capture-pose screenshot and costume data) with S3 and
// keeps only metadata itself, handing the console a presigned URL so no body ever passes
// through this process. That is exactly what nex-protocols-common-go is built for: it ships
// every base DataStore handler and leaves the operator two things — where object metadata
// lives, and where object bodies live. Here that is Postgres and an S3 bucket.
//
// "Real" here means durable Postgres metadata, a
// search that honours the whole DataStoreSearchParam, presigned S3 for bodies, and
// friends-only visibility resolved against OpenPak's actual friend graph rather than a stub.
//
// What this does NOT serve is Odyssey's own Balloon extension (0x73 methods 47-53:
// SearchBalloon, FetchMyInfos, the pose-buffer queues). Those are a title-specific protocol
// that no common library implements; see docs/design.md and docs/provenance.md.
//
// LIFT CANDIDATE. This backend is agreed to move into the shared
// github.com/openpak/nex/datastore subpackage behind a one-call store.Wire(common, host).
// Nothing here is Odyssey-shaped — the only title-specific values are the schema name and the
// S3 key bases, and the columns are a transcription of NEX's own DataStoreMetaInfo. Delete
// this file and search.go when the shared store lands. What must change on the way across is
// scoping: this uses a per-title Postgres schema, the shared store uses a title key so one
// database can hold several titles.

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	nexhost "github.com/openpak/nex"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// noPersistenceSlot is the persistence slot ID meaning "this object is not persisted".
const noPersistenceSlot = 0xFFFF

// searchLimit caps how many objects one SearchObject call may return.
const searchLimit = 100

const datastoreSchema = `
CREATE SCHEMA IF NOT EXISTS odyssey;

CREATE TABLE IF NOT EXISTS odyssey.datastore_objects (
	data_id bigserial PRIMARY KEY,
	owner_pid numeric(20) NOT NULL,
	size bigint NOT NULL DEFAULT 0,
	name text NOT NULL DEFAULT '',
	data_type int NOT NULL DEFAULT 0,
	meta_binary bytea NOT NULL DEFAULT '',
	permission int NOT NULL DEFAULT 0,
	permission_recipients numeric(20)[] NOT NULL DEFAULT '{}',
	del_permission int NOT NULL DEFAULT 0,
	del_permission_recipients numeric(20)[] NOT NULL DEFAULT '{}',
	created timestamptz NOT NULL DEFAULT now(),
	updated timestamptz NOT NULL DEFAULT now(),
	period int NOT NULL DEFAULT 0,
	status int NOT NULL DEFAULT 0,
	referred_count bigint NOT NULL DEFAULT 0,
	refer_data_id bigint NOT NULL DEFAULT 0,
	flag int NOT NULL DEFAULT 0,
	tags text[] NOT NULL DEFAULT '{}',
	persistence_slot int,
	uploaded boolean NOT NULL DEFAULT false,
	deleted boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS odyssey.datastore_ratings (
	data_id bigint NOT NULL REFERENCES odyssey.datastore_objects(data_id) ON DELETE CASCADE,
	slot smallint NOT NULL,
	total bigint NOT NULL DEFAULT 0,
	count bigint NOT NULL DEFAULT 0,
	initial bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (data_id, slot)
);
`

// objectColumns is the column list every read of an object uses, in the order scanObject
// expects them.
const objectColumns = `data_id, owner_pid, size, name, data_type, meta_binary, permission,
	permission_recipients, del_permission, del_permission_recipients, created, updated,
	period, status, referred_count, refer_data_id, flag, tags`

// store is the DataStore backing: metadata rows in Postgres, plus the friend lookup a
// friends-only search needs. Balloon World shows friends' balloons, so the friend graph is
// not optional decoration here — it is the visibility rule.
type store struct {
	db      *sql.DB
	friends func(pid uint32) []uint32
}

// registerDataStore wires the common DataStore handlers to Postgres and, if an S3 endpoint is
// configured, to that bucket. Without S3 the metadata calls (GetMeta, GetMetas, SearchObject,
// ChangeMeta, RateObject, DeleteObject, PostMetaBinary) all work and the three that move an
// object body answer Core::NotImplemented, which is what nex-protocols-common-go does for an
// unset callback anyway.
func registerDataStore(host *nexhost.Host, database *sql.DB, friends func(pid uint32) []uint32) error {
	if _, err := database.Exec(datastoreSchema); err != nil {
		return err
	}

	protocol := datastore.NewProtocol()
	host.Secure.RegisterServiceProtocol(protocol)
	common := common_datastore.NewCommonProtocol(protocol)

	s := &store{db: database, friends: friends}
	common.GetUserFriendPIDs = friends
	common.GetObjectInfoByDataID = s.objectByDataID
	common.GetObjectInfoByDataIDWithPassword = func(dataID types.UInt64, _ types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
		return s.objectByDataID(dataID)
	}
	common.GetObjectInfoByPersistenceTargetWithPassword = func(target datastore_types.DataStorePersistenceTarget, _ types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
		return s.objectByPersistenceTarget(target)
	}
	common.GetObjectOwnerByDataID = s.ownerByDataID
	common.GetObjectSizeByDataID = s.sizeByDataID
	common.InitializeObjectByPreparePostParam = s.initializeObject
	common.InitializeObjectRatingWithSlot = s.initializeRating
	common.UpdateObjectUploadCompletedByDataID = s.setUploaded
	common.UpdateObjectMetaBinaryByDataIDWithPassword = func(dataID types.UInt64, metaBinary types.QBuffer, _ types.UInt64) *nex.Error {
		return s.update(dataID, "meta_binary", []byte(metaBinary))
	}
	common.UpdateObjectPeriodByDataIDWithPassword = func(dataID types.UInt64, period types.UInt16, _ types.UInt64) *nex.Error {
		return s.update(dataID, "period", int64(period))
	}
	// The library declares these two with each other's parameter names — its "Period" callback
	// names its second argument dataType and vice versa — while ChangeMeta passes the right
	// value to each. Go by the callback name, not the parameter name.
	common.UpdateObjectDataTypeByDataIDWithPassword = func(dataID types.UInt64, dataType types.UInt16, _ types.UInt64) *nex.Error {
		return s.update(dataID, "data_type", int64(dataType))
	}
	common.DeleteObjectByDataID = s.deleteObject
	common.DeleteObjectByDataIDWithPassword = func(dataID types.UInt64, _ types.UInt64) *nex.Error {
		return s.deleteObject(dataID)
	}
	common.RateObjectWithPassword = func(dataID types.UInt64, slot types.UInt8, value types.Int32, _ types.UInt64) (datastore_types.DataStoreRatingInfo, *nex.Error) {
		return s.rate(dataID, int16(slot), int64(value))
	}
	common.GetObjectInfosByDataStoreSearchParam = s.search

	client, bucket, err := s3Client()
	if err != nil {
		return err
	}
	if client == nil {
		return nil
	}
	common.S3Bucket = bucket
	common.SetDataKeyBase("odyssey/objects")
	common.SetNotifyKeyBase("odyssey/notifications")
	common.SetMinIOClient(client)
	return nil
}

// s3Client builds the MinIO client for object bodies, or (nil, "", nil) when no endpoint is
// configured. A half-configured store is a startup error, not a silent downgrade.
func s3Client() (*minio.Client, string, error) {
	endpoint := os.Getenv("SMO_S3_ENDPOINT")
	if endpoint == "" {
		return nil, "", nil
	}
	bucket := os.Getenv("SMO_S3_BUCKET")
	access := os.Getenv("SMO_S3_ACCESS_KEY")
	secret := os.Getenv("SMO_S3_SECRET_KEY")
	if bucket == "" || access == "" || secret == "" {
		return nil, "", errRequired("SMO_S3_BUCKET, SMO_S3_ACCESS_KEY and SMO_S3_SECRET_KEY (with SMO_S3_ENDPOINT set)")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: os.Getenv("SMO_S3_INSECURE") != "1",
	})
	return client, bucket, err
}

// initializeObject creates the metadata row a PreparePostObject or PostMetaBinary needs and
// returns its data ID; the body, if there is one, arrives at S3 afterwards.
func (s *store) initializeObject(owner types.PID, param datastore_types.DataStorePreparePostParam) (uint64, *nex.Error) {
	// An object is visible as soon as it exists unless the client asked to complete it
	// separately. A meta-binary-only object has no body to wait for either way.
	uploaded := !param.Flag.HasFlag(constants.DataFlagNeedCompletion) ||
		param.Flag.HasFlag(constants.DataFlagNotUseFileServer)

	status := constants.DataStatusNone
	if param.Flag.HasFlag(constants.DataFlagNeedReview) {
		status = constants.DataStatusPending
	}

	var slot any
	if param.PersistenceInitParam.PersistenceSlotID != noPersistenceSlot {
		slot = int64(param.PersistenceInitParam.PersistenceSlotID)
	}

	transaction, err := s.db.Begin()
	if err != nil {
		return 0, storeError(err)
	}
	defer transaction.Rollback()

	if slot != nil {
		// ponytail: a new object in a persistence slot always retires the previous one,
		// whatever DeleteLastObject says — nex-protocols-common-go's own note is that the flag
		// is set even where it makes no sense. Honour the flag if a title ever turns out to
		// need two live objects in one slot.
		if _, err := transaction.Exec(
			`UPDATE odyssey.datastore_objects SET deleted = true WHERE owner_pid = $1 AND persistence_slot = $2 AND NOT deleted`,
			uint64(owner), slot,
		); err != nil {
			return 0, storeError(err)
		}
	}

	var dataID uint64
	err = transaction.QueryRow(`INSERT INTO odyssey.datastore_objects (
			owner_pid, size, name, data_type, meta_binary, permission, permission_recipients,
			del_permission, del_permission_recipients, period, status, refer_data_id, flag,
			tags, persistence_slot, uploaded
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING data_id`,
		uint64(owner), int64(param.Size), string(param.Name), int64(param.DataType),
		[]byte(param.MetaBinary), int64(param.Permission.Permission), pidArray(param.Permission.RecipientIDs),
		int64(param.DelPermission.Permission), pidArray(param.DelPermission.RecipientIDs),
		int64(param.Period), int64(status), int64(param.ReferDataID), int64(param.Flag),
		pq.Array(stringSlice(param.Tags)), slot, uploaded,
	).Scan(&dataID)
	if err != nil {
		return 0, storeError(err)
	}
	return dataID, storeError(transaction.Commit())
}

// initializeRating creates one rating slot for a freshly posted object.
func (s *store) initializeRating(dataID uint64, param datastore_types.DataStoreRatingInitParamWithSlot) *nex.Error {
	_, err := s.db.Exec(
		`INSERT INTO odyssey.datastore_ratings (data_id, slot, total, count, initial)
		 VALUES ($1,$2,$3,0,$3) ON CONFLICT (data_id, slot) DO NOTHING`,
		dataID, int16(param.Slot), int64(param.Param.InitialValue),
	)
	return storeError(err)
}

// rate applies one clap and returns the slot's new totals.
func (s *store) rate(dataID types.UInt64, slot int16, value int64) (datastore_types.DataStoreRatingInfo, *nex.Error) {
	rating := datastore_types.NewDataStoreRatingInfo()
	err := s.db.QueryRow(
		`UPDATE odyssey.datastore_ratings SET total = total + $3, count = count + 1
		 WHERE data_id = $1 AND slot = $2 RETURNING total, count, initial`,
		uint64(dataID), slot, value,
	).Scan(&rating.TotalValue, &rating.Count, &rating.InitialValue)
	if err == sql.ErrNoRows {
		return rating, nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such rating slot")
	}
	return rating, storeError(err)
}

func (s *store) objectByDataID(dataID types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.object(`data_id = $1 AND uploaded AND NOT deleted`, uint64(dataID))
}

func (s *store) objectByPersistenceTarget(target datastore_types.DataStorePersistenceTarget) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.object(
		`owner_pid = $1 AND persistence_slot = $2 AND uploaded AND NOT deleted`,
		uint64(target.OwnerID), int64(target.PersistenceSlotID),
	)
}

func (s *store) object(where string, args ...any) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	row := s.db.QueryRow(`SELECT `+objectColumns+` FROM odyssey.datastore_objects WHERE `+where, args...)
	object, err := scanObject(row, nil)
	if err == sql.ErrNoRows {
		return object, nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	if err != nil {
		return object, storeError(err)
	}
	// attachRatings fills in the slice it is given, so read the object back out of it rather
	// than off the copy that went in.
	objects := []datastore_types.DataStoreMetaInfo{object}
	if nexErr := s.attachRatings(objects); nexErr != nil {
		return object, nexErr
	}
	return objects[0], nil
}

// ownerByDataID and sizeByDataID answer for objects that have not been completed yet, which is
// the whole point of them: CompletePostObject needs both before it flips the flag.
func (s *store) ownerByDataID(dataID types.UInt64) (uint32, *nex.Error) {
	var owner uint64
	err := s.db.QueryRow(`SELECT owner_pid FROM odyssey.datastore_objects WHERE data_id = $1 AND NOT deleted`, uint64(dataID)).Scan(&owner)
	if err == sql.ErrNoRows {
		return 0, nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return uint32(owner), storeError(err)
}

func (s *store) sizeByDataID(dataID types.UInt64) (uint32, *nex.Error) {
	var size int64
	err := s.db.QueryRow(`SELECT size FROM odyssey.datastore_objects WHERE data_id = $1 AND NOT deleted`, uint64(dataID)).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return uint32(size), storeError(err)
}

func (s *store) setUploaded(dataID types.UInt64, uploaded bool) *nex.Error {
	return s.update(dataID, "uploaded", uploaded)
}

func (s *store) deleteObject(dataID types.UInt64) *nex.Error {
	return s.update(dataID, "deleted", true)
}

// update writes one column of one object. column is always a literal from this file, never
// anything a console sent.
func (s *store) update(dataID types.UInt64, column string, value any) *nex.Error {
	result, err := s.db.Exec(
		`UPDATE odyssey.datastore_objects SET `+column+` = $2, updated = now() WHERE data_id = $1 AND NOT deleted`,
		uint64(dataID), value,
	)
	if err != nil {
		return storeError(err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return nil
}

func (s *store) search(param datastore_types.DataStoreSearchParam, caller types.PID) ([]datastore_types.DataStoreMetaInfo, uint32, *nex.Error) {
	var friends []uint32
	if needsFriends(param) && s.friends != nil {
		friends = s.friends(uint32(caller))
	}

	query, args := searchQuery(param, uint64(caller), friends)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, storeError(err)
	}
	defer rows.Close()

	var objects []datastore_types.DataStoreMetaInfo
	var total uint32
	for rows.Next() {
		object, err := scanObject(rows, &total)
		if err != nil {
			return nil, 0, storeError(err)
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, storeError(err)
	}
	return objects, total, s.attachRatings(objects)
}

// attachRatings fills in every object's rating slots in one query. The caller's ResultOption
// decides whether they survive to the wire; that filtering is the library's job.
func (s *store) attachRatings(objects []datastore_types.DataStoreMetaInfo) *nex.Error {
	if len(objects) == 0 {
		return nil
	}
	ids := make([]int64, len(objects))
	for i, object := range objects {
		ids[i] = int64(object.DataID)
	}
	rows, err := s.db.Query(
		`SELECT data_id, slot, total, count, initial FROM odyssey.datastore_ratings WHERE data_id = ANY($1) ORDER BY slot`,
		pq.Array(ids),
	)
	if err != nil {
		return storeError(err)
	}
	defer rows.Close()

	byDataID := make(map[uint64]int, len(objects))
	for i, object := range objects {
		byDataID[uint64(object.DataID)] = i
	}
	for rows.Next() {
		var dataID uint64
		withSlot := datastore_types.NewDataStoreRatingInfoWithSlot()
		if err := rows.Scan(&dataID, &withSlot.Slot, &withSlot.Rating.TotalValue, &withSlot.Rating.Count, &withSlot.Rating.InitialValue); err != nil {
			return storeError(err)
		}
		if i, ok := byDataID[dataID]; ok {
			objects[i].Ratings = append(objects[i].Ratings, withSlot)
		}
	}
	return storeError(rows.Err())
}

type scanner interface{ Scan(dest ...any) error }

// scanObject reads one objectColumns row. total, when not nil, also takes the window function
// count that searchQuery appends.
func scanObject(row scanner, total *uint32) (datastore_types.DataStoreMetaInfo, error) {
	object := datastore_types.NewDataStoreMetaInfo()

	var dataID, owner uint64
	var size, dataType, permission, delPermission, period, status, referredCount, referDataID, flag int64
	var name string
	var metaBinary []byte
	var recipients, delRecipients, tags pq.StringArray
	var created, updated time.Time

	dest := []any{
		&dataID, &owner, &size, &name, &dataType, &metaBinary, &permission,
		&recipients, &delPermission, &delRecipients, &created, &updated,
		&period, &status, &referredCount, &referDataID, &flag, &tags,
	}
	if total != nil {
		dest = append(dest, total)
	}
	if err := row.Scan(dest...); err != nil {
		return object, err
	}

	object.DataID = types.NewUInt64(dataID)
	object.OwnerID = types.NewPID(owner)
	object.Size = types.NewUInt32(uint32(size))
	object.Name = types.NewString(name)
	object.DataType = types.NewUInt16(uint16(dataType))
	object.MetaBinary = types.NewQBuffer(metaBinary)
	object.Permission = permissionOf(permission, recipients)
	object.DelPermission = permissionOf(delPermission, delRecipients)
	object.CreatedTime = dateTime(created)
	object.UpdatedTime = dateTime(updated)
	object.Period = types.NewUInt16(uint16(period))
	object.Status = constants.DataStatus(status)
	object.ReferredCnt = types.NewUInt32(uint32(referredCount))
	object.ReferDataID = types.NewUInt32(uint32(referDataID))
	object.Flag = constants.DataFlag(flag)
	object.ReferredTime = object.UpdatedTime
	object.ExpireTime = expiry(created, period)
	object.Tags = types.NewList[types.String]()
	for _, tag := range tags {
		object.Tags = append(object.Tags, types.NewString(tag))
	}
	object.Ratings = types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()
	return object, nil
}

func permissionOf(value int64, recipients pq.StringArray) datastore_types.DataStorePermission {
	permission := datastore_types.NewDataStorePermission()
	permission.Permission = constants.Permission(value)
	permission.RecipientIDs = types.NewList[types.PID]()
	for _, recipient := range recipients {
		var pid uint64
		if _, err := fmt.Sscan(recipient, &pid); err == nil {
			permission.RecipientIDs = append(permission.RecipientIDs, types.NewPID(pid))
		}
	}
	return permission
}

func dateTime(t time.Time) types.DateTime {
	value := types.NewDateTime(0)
	return value.FromTimestamp(t.UTC())
}

// expiry is when the object stops being served. period is in days; 0 means never, which NEX
// spells as the far future rather than a zero timestamp.
func expiry(created time.Time, period int64) types.DateTime {
	if period == 0 {
		return dateTime(created.AddDate(100, 0, 0))
	}
	return dateTime(created.AddDate(0, 0, int(period)))
}

func pidArray(pids types.List[types.PID]) any {
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, fmt.Sprint(uint64(pid)))
	}
	return pq.Array(values)
}

func stringSlice(list types.List[types.String]) []string {
	values := make([]string, 0, len(list))
	for _, value := range list {
		values = append(values, string(value))
	}
	return values
}

// storeError turns a database failure into the one NEX code that fits: the console cannot act
// on our SQL, only on "it did not work".
func storeError(err error) *nex.Error {
	if err == nil {
		return nil
	}
	return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
}
