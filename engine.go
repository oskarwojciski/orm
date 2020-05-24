package orm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	levelHandler "github.com/apex/log/handlers/level"
	"github.com/juju/errors"

	"github.com/apex/log"
	"github.com/apex/log/handlers/text"
)

type Engine struct {
	registry                     *validatedRegistry
	dbs                          map[string]*DB
	localCache                   map[string]*LocalCache
	redis                        map[string]*RedisCache
	locks                        map[string]*Locker
	rabbitMQChannels             map[string]*rabbitMQChannel
	rabbitMQQueues               map[string]*RabbitMQQueue
	rabbitMQDelayedQueues        map[string]*RabbitMQDelayedQueue
	rabbitMQRouters              map[string]*RabbitMQRouter
	logMetaData                  map[string]interface{}
	trackedEntities              []Entity
	trackedEntitiesCounter       int
	loggers                      map[LoggerSource]*logger
	afterCommitLocalCacheSets    map[string][]interface{}
	afterCommitRedisCacheDeletes map[string][]string
	dataDogSpan                  tracer.Span
	dataDogCtx                   context.Context
}

func (e *Engine) StartDataDogHTTPAPM(request *http.Request, service string) (tracer.Span, context.Context) {
	resource := request.Method + " " + request.URL.Path
	opts := []ddtrace.StartSpanOption{
		tracer.ServiceName(service),
		tracer.ResourceName(resource),
		tracer.SpanType(ext.SpanTypeWeb),
		tracer.Tag(ext.HTTPMethod, request.Method),
		tracer.Tag(ext.HTTPURL, request.URL.Path),
		tracer.Measured(),
	}
	if spanCtx, err := tracer.Extract(tracer.HTTPHeadersCarrier(request.Header)); err == nil {
		opts = append(opts, tracer.ChildOf(spanCtx))
	}
	span, ctx := tracer.StartSpanFromContext(request.Context(), "http.request", opts...)
	e.dataDogSpan = span
	e.dataDogCtx = ctx
	return span, ctx
}

func (e *Engine) StopDataDogHTTPAPM(status int, err error) {
	e.dataDogSpan.SetTag(ext.HTTPCode, strconv.Itoa(status))
	if status >= 500 && status < 600 {
		if err != nil {
			stackParts := strings.Split(errors.ErrorStack(err), "\n")
			stack := strings.Join(stackParts[1:], "\n")
			fullStack := strings.Join(strings.Split(string(debug.Stack()), "\n")[2:], "\n")
			e.dataDogSpan.SetTag(ext.Error, 1)
			e.dataDogSpan.SetTag(ext.ErrorMsg, err.Error())
			e.dataDogSpan.SetTag(ext.ErrorDetails, fullStack)
			e.dataDogSpan.SetTag(ext.ErrorStack, stack)
			e.dataDogSpan.SetTag(ext.ErrorType, reflect.TypeOf(errors.Cause(err)).String())
		} else {
			e.dataDogSpan.SetTag(ext.Error, fmt.Errorf("%d: %s", status, http.StatusText(status)))
		}
	}
}

func (e *Engine) AddDataDogAPMLog(level log.Level, source ...LoggerSource) {
	if len(source) == 0 {
		source = []LoggerSource{LoggerSourceDB, LoggerSourceRedis, LoggerSourceRabbitMQ}
	}
	for _, s := range source {
		if s == LoggerSourceDB {
			e.AddLogger(newDBDataDogHandler(e.dataDogCtx), level, s)
		} else if s == LoggerSourceRabbitMQ {
			e.AddLogger(newRabbitMQDataDogHandler(e.dataDogCtx), level, s)
		} else if s == LoggerSourceRedis {
			e.AddLogger(newRedisDataDogHandler(e.dataDogCtx), level, s)
		}
	}
}

func (e *Engine) AddLogger(handler log.Handler, level log.Level, source ...LoggerSource) {
	if e.loggers == nil {
		e.loggers = make(map[LoggerSource]*logger)
	}
	if len(source) == 0 {
		source = []LoggerSource{LoggerSourceDB, LoggerSourceRedis, LoggerSourceRabbitMQ}
	}
	newHandler := levelHandler.New(handler, level)
	for _, source := range source {
		l, has := e.loggers[source]
		if has {
			l.handler.Handlers = append(l.handler.Handlers, newHandler)
		} else {
			e.loggers[source] = e.newLogger(newHandler, level)
		}
	}
}

func (e *Engine) EnableDebug(source ...LoggerSource) {
	e.AddLogger(text.New(os.Stdout), log.DebugLevel, source...)
}

func (e *Engine) SetLogMetaData(key string, value interface{}) {
	if e.logMetaData == nil {
		e.logMetaData = make(map[string]interface{})
	}
	e.logMetaData[key] = value
}

func (e *Engine) Track(entity ...Entity) {
	for _, entity := range entity {
		initIfNeeded(e, entity)
		e.trackedEntities = append(e.trackedEntities, entity)
		e.trackedEntitiesCounter++
		if e.trackedEntitiesCounter == 10000 {
			panic(errors.Errorf("track limit 10000 exceeded"))
		}
	}
}

func (e *Engine) TrackAndFlush(entity ...Entity) error {
	e.Track(entity...)
	return e.Flush()
}

func (e *Engine) Flush() error {
	return e.flushTrackedEntities(false, false)
}

func (e *Engine) FlushLazy() error {
	return e.flushTrackedEntities(true, false)
}

func (e *Engine) FlushInTransaction() error {
	return e.flushTrackedEntities(false, true)
}

func (e *Engine) FlushWithLock(lockerPool string, lockName string, ttl time.Duration, waitTimeout time.Duration) error {
	return e.flushWithLock(false, lockerPool, lockName, ttl, waitTimeout)
}

func (e *Engine) FlushInTransactionWithLock(lockerPool string, lockName string, ttl time.Duration, waitTimeout time.Duration) error {
	return e.flushWithLock(true, lockerPool, lockName, ttl, waitTimeout)
}

func (e *Engine) ClearTrackedEntities() {
	e.trackedEntities = make([]Entity, 0)
}

func (e *Engine) SetOnDuplicateKeyUpdate(update *Where, entity ...Entity) {
	for _, row := range entity {
		orm := initIfNeeded(e, row)
		orm.attributes.onDuplicateKeyUpdate = update
	}
}

func (e *Engine) SetEntityLogMeta(key string, value interface{}, entity ...Entity) {
	for _, row := range entity {
		orm := initIfNeeded(e, row)
		if orm.attributes.logMeta == nil {
			orm.attributes.logMeta = make(map[string]interface{})
		}
		orm.attributes.logMeta[key] = value
	}
}

func (e *Engine) MarkToDelete(entity ...Entity) {
	for _, row := range entity {
		e.Track(row)
		orm := initIfNeeded(e, row)
		if orm.tableSchema.hasFakeDelete {
			orm.attributes.elem.FieldByName("FakeDelete").SetBool(true)
			continue
		}
		orm.attributes.delete = true
	}
}

func (e *Engine) ForceMarkToDelete(entity ...Entity) {
	for _, row := range entity {
		orm := initIfNeeded(e, row)
		orm.attributes.delete = true
		e.Track(row)
	}
}

func (e *Engine) MarkDirty(entity Entity, queueCode string, ids ...uint64) error {
	_, has := e.GetRegistry().GetDirtyQueues()[queueCode]
	if !has {
		return errors.Errorf("unknown dirty queue '%s'", queueCode)
	}
	channel := e.GetRabbitMQQueue("dirty_queue_" + queueCode)
	entityName := initIfNeeded(e, entity).tableSchema.t.String()
	for _, id := range ids {
		val := &DirtyQueueValue{Updated: true, ID: id, EntityName: entityName}
		asJSON, _ := json.Marshal(val)
		err := channel.Publish(asJSON)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *Engine) Loaded(entity Entity) bool {
	orm := initIfNeeded(e, entity)
	return orm.attributes.loaded
}

func (e *Engine) IsDirty(entity Entity) bool {
	if !e.Loaded(entity) {
		return true
	}
	initIfNeeded(e, entity)
	is, _ := getDirtyBind(entity)
	return is
}

func (e *Engine) GetRegistry() ValidatedRegistry {
	return e.registry
}

func (e *Engine) GetMysql(code ...string) *DB {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	db, has := e.dbs[dbCode]
	if !has {
		panic(errors.Errorf("unregistered mysql pool '%s'", dbCode))
	}
	return db
}

func (e *Engine) GetLocalCache(code ...string) *LocalCache {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	cache, has := e.localCache[dbCode]
	if !has {
		panic(errors.Errorf("unregistered local cache pool '%s'", dbCode))
	}
	return cache
}

func (e *Engine) GetRedis(code ...string) *RedisCache {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	cache, has := e.redis[dbCode]
	if !has {
		panic(errors.Errorf("unregistered redis cache pool '%s'", dbCode))
	}
	return cache
}

func (e *Engine) GetRabbitMQQueue(queueName string) *RabbitMQQueue {
	queue, has := e.rabbitMQQueues[queueName]
	if has {
		return queue
	}
	channel, has := e.rabbitMQChannels[queueName]
	if !has {
		panic(errors.Errorf("unregistered rabbitMQ queue '%s'", queueName))
	}
	if channel.config.Router != "" {
		panic(errors.Errorf("rabbitMQ queue '%s' is declared as router", queueName))
	}
	if e.rabbitMQQueues == nil {
		e.rabbitMQQueues = make(map[string]*RabbitMQQueue)
	}
	e.rabbitMQQueues[queueName] = &RabbitMQQueue{channel}
	return e.rabbitMQQueues[queueName]
}

func (e *Engine) GetRabbitMQDelayedQueue(queueName string) *RabbitMQDelayedQueue {
	queue, has := e.rabbitMQDelayedQueues[queueName]
	if has {
		return queue
	}
	channel, has := e.rabbitMQChannels[queueName]
	if !has {
		panic(errors.Errorf("unregistered rabbitMQ delayed queue '%s'", queueName))
	}
	if channel.config.Router == "" {
		panic(errors.Errorf("rabbitMQ queue '%s' is not declared as delayed queue", queueName))
	}
	if !channel.config.Delayed {
		panic(errors.Errorf("rabbitMQ queue '%s' is not declared as delayed queue", queueName))
	}
	if e.rabbitMQDelayedQueues == nil {
		e.rabbitMQDelayedQueues = make(map[string]*RabbitMQDelayedQueue)
	}
	e.rabbitMQDelayedQueues[queueName] = &RabbitMQDelayedQueue{channel}
	return e.rabbitMQDelayedQueues[queueName]
}

func (e *Engine) GetRabbitMQRouter(channelName string) *RabbitMQRouter {
	queue, has := e.rabbitMQRouters[channelName]
	if has {
		return queue
	}
	channel, has := e.rabbitMQChannels[channelName]
	if !has {
		panic(errors.Errorf("unregistered rabbitMQ router '%s'", channelName))
	}
	if channel.config.Router == "" {
		panic(errors.Errorf("rabbitMQ queue '%s' is not declared as router", channelName))
	}
	if channel.config.Delayed {
		panic(errors.Errorf("rabbitMQ queue '%s' is declared as delayed queue", channelName))
	}
	if e.rabbitMQRouters == nil {
		e.rabbitMQRouters = make(map[string]*RabbitMQRouter)
	}
	e.rabbitMQRouters[channelName] = &RabbitMQRouter{channel}
	return e.rabbitMQRouters[channelName]
}

func (e *Engine) GetLocker(code ...string) *Locker {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	locker, has := e.locks[dbCode]
	if !has {
		panic(errors.Errorf("unregistered locker pool '%s'", dbCode))
	}
	return locker
}

func (e *Engine) SearchWithCount(where *Where, pager *Pager, entities interface{}, references ...string) (totalRows int, err error) {
	return search(true, e, where, pager, true, reflect.ValueOf(entities).Elem(), references...)
}

func (e *Engine) Search(where *Where, pager *Pager, entities interface{}, references ...string) error {
	_, err := search(true, e, where, pager, false, reflect.ValueOf(entities).Elem(), references...)
	return errors.Trace(err)
}

func (e *Engine) SearchIDsWithCount(where *Where, pager *Pager, entity interface{}) (results []uint64, totalRows int, err error) {
	return searchIDsWithCount(true, e, where, pager, reflect.TypeOf(entity))
}

func (e *Engine) SearchIDs(where *Where, pager *Pager, entity Entity) ([]uint64, error) {
	results, _, err := searchIDs(true, e, where, pager, false, reflect.TypeOf(entity).Elem())
	return results, errors.Trace(err)
}

func (e *Engine) SearchOne(where *Where, entity Entity, references ...string) (bool, error) {
	return searchOne(true, e, where, entity, references)
}

func (e *Engine) CachedSearchOne(entity Entity, indexName string, arguments ...interface{}) (has bool, err error) {
	return cachedSearchOne(e, entity, indexName, arguments, nil)
}

func (e *Engine) CachedSearchOneWithReferences(entity Entity, indexName string, arguments []interface{}, references []string) (has bool, err error) {
	return cachedSearchOne(e, entity, indexName, arguments, references)
}

func (e *Engine) CachedSearch(entities interface{}, indexName string, pager *Pager, arguments ...interface{}) (totalRows int, err error) {
	return cachedSearch(e, entities, indexName, pager, arguments, nil)
}

func (e *Engine) CachedSearchWithReferences(entities interface{}, indexName string, pager *Pager,
	arguments []interface{}, references []string) (totalRows int, err error) {
	return cachedSearch(e, entities, indexName, pager, arguments, references)
}

func (e *Engine) ClearByIDs(entity Entity, ids ...uint64) error {
	return clearByIDs(e, entity, ids...)
}

func (e *Engine) FlushInCache(entities ...Entity) error {
	return flushInCache(e, entities...)
}

func (e *Engine) LoadByID(id uint64, entity Entity, references ...string) (found bool, err error) {
	return loadByID(e, id, entity, true, references...)
}

func (e *Engine) Load(entity Entity, references ...string) error {
	if e.Loaded(entity) {
		if len(references) > 0 {
			orm := entity.getORM()
			return warmUpReferences(e, orm.tableSchema, orm.attributes.elem, references, false)
		}
		return nil
	}
	orm := initIfNeeded(e, entity)
	id := orm.GetID()
	if id > 0 {
		_, err := loadByID(e, id, entity, true, references...)
		return errors.Trace(err)
	}
	return nil
}

func (e *Engine) LoadByIDs(ids []uint64, entities interface{}, references ...string) (missing []uint64, err error) {
	return tryByIDs(e, ids, reflect.ValueOf(entities).Elem(), references)
}

func (e *Engine) GetAlters() (alters []Alter, err error) {
	return getAlters(e)
}

func (e *Engine) flushTrackedEntities(lazy bool, transaction bool) error {
	if e.trackedEntitiesCounter == 0 {
		return nil
	}
	var dbPools map[string]*DB
	if transaction {
		dbPools = make(map[string]*DB)
		for _, entity := range e.trackedEntities {
			db := entity.getORM().tableSchema.GetMysql(e)
			dbPools[db.code] = db
		}
		for _, db := range dbPools {
			err := db.Begin()
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	defer func() {
		for _, db := range dbPools {
			db.Rollback()
		}
	}()

	err := flush(e, lazy, transaction, e.trackedEntities...)
	if err != nil {
		return errors.Trace(err)
	}
	if transaction {
		for _, db := range dbPools {
			err := db.Commit()
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	e.trackedEntities = make([]Entity, 0)
	e.trackedEntitiesCounter = 0
	return nil
}

func (e *Engine) flushWithLock(transaction bool, lockerPool string, lockName string, ttl time.Duration, waitTimeout time.Duration) error {
	locker := e.GetLocker(lockerPool)
	lock, has, err := locker.Obtain(lockName, ttl, waitTimeout)
	if err != nil {
		return errors.Trace(err)
	}
	if !has {
		return errors.Errorf("lock wait timeout")
	}
	defer lock.Release()
	return e.flushTrackedEntities(false, transaction)
}
