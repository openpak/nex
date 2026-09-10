package smm2

// DataStore (protocol 0x73) storage — the base object layer.
//
// For Super Mario Maker 2 a DataStore object is a COURSE: its metadata (title, data
// type, tags, ratings) lives in Postgres, and its level BLOB — the thing the console
// PUTs and GETs during a course upload/download — lives in an S3 bucket reached through
// presigned URLs the library builds itself. A player's own courses occupy persistence
// slots (see clearSlot); the game caps a maker at 32 published courses, which is the
// natural slot ceiling.
//
// This wires the STANDARD DataStore methods nex-protocols-common-go implements:
// PreparePostObject (24), PrepareGetObject (25), CompletePostObject (26), SearchObject,
// GetMeta, RateObject and the rest. SMM2 also speaks a large set of TITLE-SPECIFIC 0x73
// methods (RegisterUser 47, GetUsers 48, SyncUserProfile 49, CanPostCourse 60, the
// Course World browse/comment/world-map methods) that are NOT part of the common
// implementation. Those are the shared-store layer's remit; see docs/design.md and
// docs/provenance.md for the full method map recorded from the facts.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// noPersistenceSlot is the slot id meaning "this object is not persistent".
const noPersistenceSlot = 0xFFFF

type objectStore struct{ db *sql.DB }

func (s objectStore) wire(common *common_datastore.CommonProtocol, friends func(pid uint32) []uint32) {
	common.GetUserFriendPIDs = friends
	common.InitializeObjectByPreparePostParam = s.createObject
	common.InitializeObjectRatingWithSlot = s.createRating
	common.GetObjectInfoByDataID = s.object
	common.GetObjectInfoByDataIDWithPassword = s.objectWithPassword
	common.GetObjectInfoByPersistenceTargetWithPassword = s.objectBySlot
	common.GetObjectOwnerByDataID = s.owner
	common.GetObjectSizeByDataID = s.size
	common.UpdateObjectUploadCompletedByDataID = s.setUploadCompleted
	common.UpdateObjectPeriodByDataIDWithPassword = s.setPeriod
	common.UpdateObjectMetaBinaryByDataIDWithPassword = s.setMetaBinary
	common.UpdateObjectDataTypeByDataIDWithPassword = s.setDataType
	common.DeleteObjectByDataID = s.deleteObject
	common.DeleteObjectByDataIDWithPassword = s.deleteObjectWithPassword
	common.RateObjectWithPassword = s.rateObject
	common.GetObjectInfosByDataStoreSearchParam = s.search

	if err := connectBucket(common); err != nil {
		// Object bodies are optional: a title that only posts metadata never reaches
		// this path. Say so once at startup instead of failing a login later.
		common.S3Presigner = noBucket{reason: err}
		fmt.Println("datastore: no object bucket configured, metadata only:", err)
	}
}

// connectBucket points the library's presigner at the OpenPak object store. The
// library builds and signs the URLs; all it needs from us is a client and a bucket.
func connectBucket(common *common_datastore.CommonProtocol) error {
	endpoint := os.Getenv("DATASTORE_S3_ENDPOINT")
	if endpoint == "" {
		return errors.New("DATASTORE_S3_ENDPOINT is not set")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("DATASTORE_S3_ACCESS_KEY"), os.Getenv("DATASTORE_S3_SECRET_KEY"), ""),
		Secure: os.Getenv("DATASTORE_S3_INSECURE") == "",
	})
	if err != nil {
		return err
	}
	common.SetMinIOClient(client)
	common.S3Bucket = os.Getenv("DATASTORE_S3_BUCKET")
	common.SetDataKeyBase("super-mario-maker-2")
	common.SetNotifyKeyBase("super-mario-maker-2-notify")
	return nil
}

// noBucket answers the presigner interface when no object store is configured, so a
// title that unexpectedly uploads an object body gets an error it can report rather
// than a crashed server.
type noBucket struct{ reason error }

func (n noBucket) GetObject(string, string, time.Duration) (*url.URL, error) {
	return nil, n.reason
}

func (n noBucket) PostObject(string, string, time.Duration) (*url.URL, map[string]string, error) {
	return nil, nil, n.reason
}

// metaColumns is the object row in the order scanObject reads it.
const metaColumns = `data_id, owner_pid, size, name, data_type, meta_binary, permission, permission_recipients,
	del_permission, del_recipients, created_time, updated_time, period, status, referred_cnt, refer_data_id,
	flag, referred_time, expire_time, tags`

// createObject writes the metadata of a new object and returns its data id. This is
// what both PreparePostObject and PostMetaBinary come down to.
func (s objectStore) createObject(ownerPID types.PID, param datastore_types.DataStorePreparePostParam) (uint64, *nex.Error) {
	slot := any(nil)
	if uint16(param.PersistenceInitParam.PersistenceSlotID) != noPersistenceSlot {
		slot = int(param.PersistenceInitParam.PersistenceSlotID)
		if err := s.clearSlot(ownerPID, slot, bool(param.PersistenceInitParam.DeleteLastObject)); err != nil {
			return 0, storeError(nex.ResultCodes.DataStore.OperationNotAllowed, err)
		}
	}

	var dataID uint64
	err := s.db.QueryRow(`INSERT INTO datastore.objects
		(owner_pid, size, name, data_type, meta_binary, permission, permission_recipients,
		 del_permission, del_recipients, period, refer_data_id, flag, tags, persistence_slot,
		 created_time, updated_time, upload_completed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, now(), now(), $15)
		RETURNING data_id`,
		ownerPID, uint32(param.Size), string(param.Name), uint16(param.DataType), []byte(param.MetaBinary),
		uint8(param.Permission.Permission), pidArray(param.Permission.RecipientIDs),
		uint8(param.DelPermission.Permission), pidArray(param.DelPermission.RecipientIDs),
		uint16(param.Period), uint32(param.ReferDataID), uint8(param.Flag),
		pq.Array(stringList(param.Tags)), slot, param.Size == 0,
	).Scan(&dataID)
	if err != nil {
		return 0, storeError(nex.ResultCodes.DataStore.OperationNotAllowed, err)
	}
	return dataID, nil
}

// clearSlot makes room for a new object in a persistence slot: the previous occupant
// stops being the slot's object, and is deleted outright if the client asked for that.
func (s objectStore) clearSlot(ownerPID types.PID, slot any, deleteLast bool) error {
	statement := `UPDATE datastore.objects SET persistence_slot = NULL WHERE owner_pid = $1 AND persistence_slot = $2`
	if deleteLast {
		statement = `UPDATE datastore.objects SET persistence_slot = NULL, deleted = true WHERE owner_pid = $1 AND persistence_slot = $2`
	}
	_, err := s.db.Exec(statement, ownerPID, slot)
	return err
}

func (s objectStore) createRating(dataID uint64, param datastore_types.DataStoreRatingInitParamWithSlot) *nex.Error {
	_, err := s.db.Exec(`INSERT INTO datastore.ratings
		(data_id, slot, total_value, count, initial_value, range_min, range_max, lock_type, flag, internal_flag, period_hour, period_duration)
		VALUES ($1, $2, $3, 0, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (data_id, slot) DO NOTHING`,
		dataID, int8(param.Slot), int64(param.Param.InitialValue),
		int32(param.Param.RangeMin), int32(param.Param.RangeMax), uint8(param.Param.LockType),
		uint8(param.Param.Flag), uint8(param.Param.InternalFlag), int8(param.Param.PeriodHour), uint8(param.Param.PeriodDuration))
	if err != nil {
		return storeError(nex.ResultCodes.DataStore.OperationNotAllowed, err)
	}
	return nil
}

func (s objectStore) object(dataID types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.objectWhere(`data_id = $1`, dataID)
}

// objectWithPassword is the same lookup with the access password the client sent. An
// object stored without a password (every object this title posts) accepts any.
//
// ponytail: passwords are never set, because nothing sets one. Enforce properly when
// a title actually posts an object with a password info structure.
func (s objectStore) objectWithPassword(dataID types.UInt64, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.objectWhere(`data_id = $1 AND (access_password = 0 OR access_password = $2)`, dataID, password)
}

func (s objectStore) objectBySlot(target datastore_types.DataStorePersistenceTarget, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	return s.objectWhere(`owner_pid = $1 AND persistence_slot = $2 AND (access_password = 0 OR access_password = $3)`,
		target.OwnerID, int(target.PersistenceSlotID), password)
}

func (s objectStore) objectWhere(where string, args ...any) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	meta, err := scanObject(s.db.QueryRow(`SELECT `+metaColumns+` FROM datastore.objects WHERE deleted = false AND `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return meta, nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	if err != nil {
		return meta, storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	ratings, nexError := s.ratings(uint64(meta.DataID))
	if nexError != nil {
		return meta, nexError
	}
	meta.Ratings = ratings
	return meta, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanObject(row scanner) (datastore_types.DataStoreMetaInfo, error) {
	meta := datastore_types.NewDataStoreMetaInfo()
	var permission, delPermission uint8
	var recipients, delRecipients pq.Int64Array
	var tags pq.StringArray
	var status, flag uint8

	err := row.Scan(&meta.DataID, &meta.OwnerID, &meta.Size, &meta.Name, &meta.DataType, &meta.MetaBinary,
		&permission, &recipients, &delPermission, &delRecipients, &meta.CreatedTime, &meta.UpdatedTime,
		&meta.Period, &status, &meta.ReferredCnt, &meta.ReferDataID, &flag, &meta.ReferredTime,
		&meta.ExpireTime, &tags)
	if err != nil {
		return meta, err
	}

	meta.Permission.Permission = constants.Permission(permission)
	meta.Permission.RecipientIDs = pidList(recipients)
	meta.DelPermission.Permission = constants.Permission(delPermission)
	meta.DelPermission.RecipientIDs = pidList(delRecipients)
	meta.Status = constants.DataStatus(status)
	meta.Flag = constants.DataFlag(flag)
	meta.Tags = make(types.List[types.String], 0, len(tags))
	for _, tag := range tags {
		meta.Tags = append(meta.Tags, types.NewString(tag))
	}
	return meta, nil
}

func (s objectStore) ratings(dataID uint64) (types.List[datastore_types.DataStoreRatingInfoWithSlot], *nex.Error) {
	rows, err := s.db.Query(`SELECT slot, total_value, count, initial_value FROM datastore.ratings WHERE data_id = $1 ORDER BY slot`, dataID)
	if err != nil {
		return nil, storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	defer rows.Close()

	ratings := types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()
	for rows.Next() {
		rating := datastore_types.NewDataStoreRatingInfoWithSlot()
		if err := rows.Scan(&rating.Slot, &rating.Rating.TotalValue, &rating.Rating.Count, &rating.Rating.InitialValue); err != nil {
			return nil, storeError(nex.ResultCodes.DataStore.Unknown, err)
		}
		ratings = append(ratings, rating)
	}
	if err := rows.Err(); err != nil {
		return nil, storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	return ratings, nil
}

func (s objectStore) owner(dataID types.UInt64) (uint32, *nex.Error) {
	var owner uint64
	if err := s.db.QueryRow(`SELECT owner_pid FROM datastore.objects WHERE data_id = $1 AND deleted = false`, dataID).Scan(&owner); err != nil {
		return 0, notFoundOr(err)
	}
	return uint32(owner), nil
}

func (s objectStore) size(dataID types.UInt64) (uint32, *nex.Error) {
	var size uint32
	if err := s.db.QueryRow(`SELECT size FROM datastore.objects WHERE data_id = $1 AND deleted = false`, dataID).Scan(&size); err != nil {
		return 0, notFoundOr(err)
	}
	return size, nil
}

func (s objectStore) setUploadCompleted(dataID types.UInt64, completed bool) *nex.Error {
	return s.update(`UPDATE datastore.objects SET upload_completed = $2, updated_time = now() WHERE data_id = $1`, dataID, completed)
}

func (s objectStore) setPeriod(dataID types.UInt64, period types.UInt16, password types.UInt64) *nex.Error {
	return s.update(`UPDATE datastore.objects SET period = $2, updated_time = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`, dataID, uint16(period), password)
}

func (s objectStore) setMetaBinary(dataID types.UInt64, metaBinary types.QBuffer, password types.UInt64) *nex.Error {
	return s.update(`UPDATE datastore.objects SET meta_binary = $2, updated_time = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`, dataID, []byte(metaBinary), password)
}

// setDataType changes an object's data type. The library's parameter names say
// "period" for the second argument; the value it passes is the data type.
func (s objectStore) setDataType(dataID types.UInt64, dataType types.UInt16, password types.UInt64) *nex.Error {
	return s.update(`UPDATE datastore.objects SET data_type = $2, updated_time = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`, dataID, uint16(dataType), password)
}

func (s objectStore) deleteObject(dataID types.UInt64) *nex.Error {
	return s.update(`UPDATE datastore.objects SET deleted = true WHERE data_id = $1`, dataID)
}

func (s objectStore) deleteObjectWithPassword(dataID types.UInt64, password types.UInt64) *nex.Error {
	return s.update(`UPDATE datastore.objects SET deleted = true
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $2)`, dataID, password)
}

// update runs a statement that must touch exactly one object, and turns "touched
// none" into NotFound rather than a silent success.
func (s objectStore) update(statement string, args ...any) *nex.Error {
	result, err := s.db.Exec(statement, args...)
	if err != nil {
		return storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	if changed == 0 {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return nil
}

// rateObject adds one rating to a slot and returns the slot's running total, which is
// what the client displays.
//
// ponytail: no per-player rating history and no range check, so a player can rate the
// same object twice. Add a (data_id, slot, pid) table if a title ever shows ratings
// that matter.
func (s objectStore) rateObject(dataID types.UInt64, slot types.UInt8, ratingValue types.Int32, password types.UInt64) (datastore_types.DataStoreRatingInfo, *nex.Error) {
	rating := datastore_types.NewDataStoreRatingInfo()
	err := s.db.QueryRow(`UPDATE datastore.ratings SET total_value = total_value + $3, count = count + 1
		WHERE data_id = $1 AND slot = $2
		RETURNING total_value, count, initial_value`, dataID, uint8(slot), int32(ratingValue)).
		Scan(&rating.TotalValue, &rating.Count, &rating.InitialValue)
	if err != nil {
		return rating, notFoundOr(err)
	}
	return rating, nil
}

// search answers SearchObject. The window the client asked for comes back as rows,
// the size of the whole match as the count beside them.
func (s objectStore) search(param datastore_types.DataStoreSearchParam, pid types.PID) ([]datastore_types.DataStoreMetaInfo, uint32, *nex.Error) {
	where, args := searchConditions(param)
	limit := int(param.ResultRange.Length)
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	query := `SELECT ` + metaColumns + `, count(*) OVER () FROM datastore.objects WHERE ` + where +
		` ORDER BY ` + searchOrder(param.ResultOrderColumn, param.ResultOrder) +
		fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, uint32(param.ResultRange.Offset))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	defer rows.Close()

	objects := make([]datastore_types.DataStoreMetaInfo, 0, limit)
	var total uint32
	for rows.Next() {
		meta, err := scanObject(countingRow{rows: rows, total: &total})
		if err != nil {
			return nil, 0, storeError(nex.ResultCodes.DataStore.Unknown, err)
		}
		objects = append(objects, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, storeError(nex.ResultCodes.DataStore.Unknown, err)
	}
	return objects, total, nil
}

// countingRow lets scanObject read the object columns while the total count that
// rides along on every search row is taken off the end.
type countingRow struct {
	rows  *sql.Rows
	total *uint32
}

func (c countingRow) Scan(dest ...any) error {
	return c.rows.Scan(append(dest, c.total)...)
}

// searchConditions turns a search parameter into a WHERE clause and its arguments.
// Only the filters a client can actually set are honoured; an unset filter is absent
// from the clause rather than matched against a zero value.
func searchConditions(param datastore_types.DataStoreSearchParam) (string, []any) {
	conditions := []string{"deleted = false"}
	var args []any

	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}

	if len(param.OwnerIDs) > 0 {
		add("owner_pid = ANY($%d)", pidArray(param.OwnerIDs))
	}
	if len(param.DataTypes) > 0 {
		dataTypes := make([]int64, 0, len(param.DataTypes))
		for _, dataType := range param.DataTypes {
			dataTypes = append(dataTypes, int64(dataType))
		}
		add("data_type = ANY($%d)", pq.Array(dataTypes))
	} else if param.DataType != 0 {
		add("data_type = $%d", uint16(param.DataType))
	}
	if len(param.Tags) > 0 {
		add("tags && $%d", pq.Array(stringList(param.Tags)))
	}
	if param.ReferDataID != 0 {
		add("refer_data_id = $%d", uint32(param.ReferDataID))
	}
	if param.CreatedAfter != 0 {
		add("created_time >= $%d", param.CreatedAfter.Standard())
	}
	if param.CreatedBefore != 0 {
		add("created_time < $%d", param.CreatedBefore.Standard())
	}
	if param.UpdatedAfter != 0 {
		add("updated_time >= $%d", param.UpdatedAfter.Standard())
	}
	if param.UpdatedBefore != 0 {
		add("updated_time < $%d", param.UpdatedBefore.Standard())
	}
	return strings.Join(conditions, " AND "), args
}

// searchOrder is the ORDER BY for a search. An unknown sort column falls back to the
// data id, which is the one column every object has and the order objects were made in.
func searchOrder(column constants.SearchSortColumn, order constants.SearchSortOrder) string {
	sortColumn := "data_id"
	switch column {
	case constants.SearchSortColumnNameAlphabetical:
		sortColumn = "name"
	case constants.SearchSortColumnSize:
		sortColumn = "size"
	case constants.SearchSortColumnCreatedTime:
		sortColumn = "created_time"
	case constants.SearchSortColumnUpdatedTime:
		sortColumn = "updated_time"
	case constants.SearchSortColumnDataType:
		sortColumn = "data_type"
	}
	if order == constants.SearchSortOrderDesc {
		return sortColumn + " DESC"
	}
	return sortColumn + " ASC"
}

func notFoundOr(err error) *nex.Error {
	if errors.Is(err, sql.ErrNoRows) {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return storeError(nex.ResultCodes.DataStore.Unknown, err)
}

func pidArray(pids types.List[types.PID]) any {
	values := make([]int64, 0, len(pids))
	for _, pid := range pids {
		values = append(values, int64(pid))
	}
	return pq.Array(values)
}

func pidList(values pq.Int64Array) types.List[types.PID] {
	pids := make(types.List[types.PID], 0, len(values))
	for _, value := range values {
		pids = append(pids, types.NewPID(uint64(value)))
	}
	return pids
}

func stringList(list types.List[types.String]) []string {
	values := make([]string, 0, len(list))
	for _, value := range list {
		values = append(values, string(value))
	}
	return values
}
