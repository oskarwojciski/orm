package orm

//
//import (
//	"fmt"
//	"reflect"
//	"strconv"
//	"strings"
//)
//
//type FlushInCacheReceiver struct {
//	RedisName string
//}
//
//func (r FlushInCacheReceiver) Size() int64 {
//	return GetRedisCache(r.RedisName).ZCard("dirty_queue")
//}
//
//func (r FlushInCacheReceiver) Digest() error {
//	cache := GetRedisCache(r.RedisName)
//	for {
//		values := cache.ZPopMin("dirty_queue", 1)
//		if len(values) == 0 {
//			break
//		}
//		val := strings.Split(values[0].Member.(string), ":")
//		if len(val) != 2 {
//			continue
//		}
//		id, err := strconv.ParseUint(val[1], 10, 64)
//		if err != nil {
//			continue
//		}
//		schema := GetTableSchema(val[0])
//		cache := schema.GetRedisCacheContainer()
//		if cache == nil {
//			continue
//		}
//		cacheKey := schema.getCacheKey(id)
//		inCache, ok := cache.Get(cacheKey)
//		if !ok {
//			continue
//		}
//		entityInCache := reflect.New(schema.t).Interface()
//		fillFromDBRow(inCache, &entityInCache)
//		entityInDB := reflect.New(schema.t).Interface()
//		SearchOne(NewWhere("`Id` = ?", id), &entityInDB)
//		if entityInDB == nil {
//			continue
//		}
//		ormFieldCache := reflect.ValueOf(entityInCache).Field(0).Interface().(ORM)
//		ormFieldDB := reflect.ValueOf(entityInDB).Field(0).Interface().(ORM)
//		newData := make(map[string]interface{}, len(ormFieldCache.DBData))
//		for k, v := range ormFieldCache.DBData {
//			newData[k] = v
//		}
//		for k, v := range ormFieldDB.DBData {
//			ormFieldCache.DBData[k] = v
//		}
//		is, bind := IsDirty(entityInCache)
//		if !is {
//			continue
//		}
//
//		bindLength := len(bind)
//		fields := make([]string, bindLength)
//		attributes := make([]interface{}, bindLength+1)
//		i := 0
//		for key, value := range bind {
//			fields[i] = fmt.Sprintf("`%s` = ?", key)
//			attributes[i] = value
//			i++
//		}
//		attributes[i] = id
//		db := schema.GetMysqlDB()
//		sql := fmt.Sprintf("UPDATE %s SET %s WHERE `Id` = ?", schema.TableName, strings.Join(fields, ","))
//		_, err = db.Exec(sql, attributes...)
//		if err != nil {
//			GetRedisCache(queueRedisName).ZAdd("dirty_queue", createDirtyQueueMember(val[0], id))
//			return err
//		}
//		cacheKeys := getCacheQueriesKeys(schema, bind, ormFieldCache.DBData, false)
//		cacheKeys = append(cacheKeys, getCacheQueriesKeys(schema, bind, newData, false)...)
//		if len(cacheKeys) > 0 {
//			err = cache.Del(cacheKeys...)
//			if err != nil {
//				GetRedisCache(queueRedisName).ZAdd("dirty_queue", createDirtyQueueMember(val[0], id))
//				return err
//			}
//		}
//	}
//	return nil
//}