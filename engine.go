package orm

import (
	"container/list"
	"reflect"
)

type Engine struct {
	config     *Config
	dbs        map[string]*DB
	localCache map[string]*LocalCache
	redis      map[string]*RedisCache
}

func NewEngine(config *Config) *Engine {
	e := &Engine{config: config}
	e.dbs = make(map[string]*DB)
	if e.config.sqlClients != nil {
		for key, val := range e.config.sqlClients {
			e.dbs[key] = &DB{engine: e, code: val.code, databaseName: val.databaseName, db: val.db}
		}
	}
	e.localCache = make(map[string]*LocalCache)
	if e.config.localCacheContainers != nil {
		for key, val := range e.config.localCacheContainers {
			e.localCache[key] = &LocalCache{engine: e, code: val.code, lru: val.lru, ttl: val.ttl}
		}
	}
	e.redis = make(map[string]*RedisCache)
	if e.config.redisServers != nil {
		for key, val := range e.config.redisServers {
			e.redis[key] = &RedisCache{engine: e, code: val.code, client: val.client}
		}
	}
	return e
}

func (e *Engine) GetConfig() *Config {
	return e.config
}

func (e *Engine) GetMysql(code ...string) (db *DB, has bool) {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	db, has = e.dbs[dbCode]
	return db, has
}

func (e *Engine) GetLocalCache(code ...string) (cache *LocalCache, has bool) {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	cache, has = e.localCache[dbCode]
	return cache, has
}

func (e *Engine) GetRedis(code ...string) (cache *RedisCache, has bool) {
	dbCode := "default"
	if len(code) > 0 {
		dbCode = code[0]
	}
	cache, has = e.redis[dbCode]
	return cache, has
}

func (e *Engine) IsDirty(entity interface{}) bool {
	is, _, _ := isDirty(reflect.ValueOf(entity).Elem())
	return is
}

func (e *Engine) Init(entity ...interface{}) error {
	return e.initEntities(entity...)
}

func (e *Engine) Flush(entities ...interface{}) error {
	return flush(e, false, entities...)
}

func (e *Engine) FlushLazy(entities ...interface{}) error {
	return flush(e, true, entities...)
}

func (e *Engine) SearchWithCount(where *Where, pager *Pager, entities interface{}, references ...string) (totalRows int, err error) {
	return search(e, where, pager, true, reflect.ValueOf(entities).Elem(), references...)
}

func (e *Engine) Search(where *Where, pager *Pager, entities interface{}, references ...string) error {
	_, err := search(e, where, pager, false, reflect.ValueOf(entities).Elem(), references...)
	return err
}

func (e *Engine) SearchIdsWithCount(where *Where, pager *Pager, entity interface{}) (results []uint64, totalRows int, err error) {
	return searchIdsWithCount(e, where, pager, reflect.TypeOf(entity))
}

func (e *Engine) SearchIds(where *Where, pager *Pager, entity interface{}) ([]uint64, error) {
	results, _, err := searchIds(e, where, pager, false, reflect.TypeOf(entity))
	return results, err
}

func (e *Engine) SearchOne(where *Where, entity interface{}) (bool, error) {
	return searchOne(e, where, entity)
}

func (e *Engine) GetByIds(ids []uint64, entities interface{}, references ...string) error {
	return getByIds(e, ids, entities, references...)
}

func (e *Engine) TryByIds(ids []uint64, entities interface{}, references ...string) (missing []uint64, err error) {
	return tryByIds(e, ids, reflect.ValueOf(entities).Elem(), references)
}

func (e *Engine) ClearCachedSearchOne(entity interface{}, indexName string, arguments ...interface{}) error {
	_, err := cachedSearchOne(e, entity, indexName, true, arguments...)
	return err
}

func (e *Engine) CachedSearchOne(entity interface{}, indexName string, arguments ...interface{}) (has bool, err error) {
	return cachedSearchOne(e, entity, indexName, false, arguments...)
}

func (e *Engine) ClearCachedSearch(entities interface{}, indexName string, pager *Pager, arguments ...interface{}) (totalRows int, err error) {
	return cachedSearch(e, entities, indexName, true, pager, arguments...)
}

func (e *Engine) CachedSearch(entities interface{}, indexName string, pager *Pager, arguments ...interface{}) (totalRows int, err error) {
	return cachedSearch(e, entities, indexName, false, pager, arguments...)
}

func (e *Engine) ClearByIds(entity interface{}, ids ...uint64) error {
	return clearByIds(e, entity, ids...)
}

func (e *Engine) FlushInCache(entities ...interface{}) error {
	return flushInCache(e, entities...)
}

func (e *Engine) TryById(id uint64, entity interface{}, references ...string) (found bool, err error) {
	return tryById(e, id, entity, references...)
}

func (e *Engine) GetById(id uint64, entity interface{}, references ...string) error {
	return getById(e, id, entity, references...)
}

func (e *Engine) GetAlters() (alters []Alter, err error) {
	return getAlters(e)
}

func (e *Engine) RegisterDatabaseLogger(logger DatabaseLogger) []*list.Element {
	res := make([]*list.Element, 0)
	for _, db := range e.dbs {
		res = append(res, db.RegisterLogger(logger))
	}
	return res
}

func (e *Engine) RegisterRedisLogger(logger CacheLogger) []*list.Element {
	res := make([]*list.Element, 0)
	for _, red := range e.redis {
		res = append(res, red.RegisterLogger(logger))
	}
	return res
}

func (e *Engine) initIfNeeded(value reflect.Value) (*ORM, error) {
	elem := value.Elem()
	orm := elem.Field(0).Interface().(*ORM)
	if orm == nil {
		tableSchema, has, err := getTableSchema(e.config, elem.Type())
		if err != nil {
			return nil, err
		}
		if !has {
			return nil, EntityNotRegisteredError{Name: elem.Type().String()}
		}
		orm = &ORM{dBData: make(map[string]interface{}), elem: elem, tableSchema: tableSchema}
		elem.Field(0).Set(reflect.ValueOf(orm))
		for _, code := range tableSchema.refOne {
			reference := tableSchema.Tags[code]["ref"]
			t, has := e.config.getEntityType(reference)
			if !has {
				return nil, EntityNotRegisteredError{Name: elem.Type().String()}
			}
			def := ReferenceOne{t: t}
			elem.FieldByName(code).Set(reflect.ValueOf(&def))
		}
		defaultInterface, is := value.Interface().(DefaultValuesInterface)
		if is {
			defaultInterface.SetDefaults()
		}
	}
	return orm, nil
}

func (e *Engine) initEntities(entity ...interface{}) error {
	for _, entity := range entity {
		value := reflect.ValueOf(entity)
		_, err := e.initIfNeeded(value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) getRedisForQueue(code string) (*RedisCache, bool) {
	return e.GetRedis(code + "_queue")
}
