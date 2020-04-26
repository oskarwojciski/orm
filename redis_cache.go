package orm

import (
	"encoding/json"
	"fmt"
	"github.com/apex/log"
	"github.com/apex/log/handlers/multi"
	"github.com/apex/log/handlers/text"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
)

type RedisCache struct {
	engine  *Engine
	code    string
	client  *redis.Client
	loggers []CacheLogger
	log          *log.Entry
	logHandler   *multi.Handler
}

type GetSetProvider func() interface{}

func (r *RedisCache) AddLogger(handler log.Handler) {
	r.logHandler.Handlers = append(r.logHandler.Handlers, handler)
}

func (r *RedisCache) SetLogLevel(level log.Level) {
	logger := log.Logger{Handler: r.logHandler, Level: level}
	r.log = logger.WithField("source", "orm")
}

func (r *RedisCache) EnableDebug() {
	r.AddLogger(text.New(os.Stdout))
	r.SetLogLevel(log.DebugLevel)
}

func (r *RedisCache) GetSet(key string, ttlSeconds int, provider GetSetProvider) (interface{}, error) {
	val, has, err := r.Get(key)
	if err != nil {
		return nil, err
	}
	if !has {
		userVal := provider()
		encoded, err := json.Marshal(userVal)
		if err != nil {
			return nil, err
		}
		return userVal, r.Set(key, string(encoded), ttlSeconds)
	}
	var data interface{}
	err = json.Unmarshal([]byte(val), &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r *RedisCache) Get(key string) (value string, has bool, err error) {
	start := time.Now()
	val, err := r.client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			if r.log != nil {
				r.fillLogFields(start, "get", 1).WithField("Key", key).Info("[ORM][REDIS][GET]")
			}
			r.logOld(key, "GET", time.Since(start).Microseconds(), 1)
			return "", false, nil
		}
		return "", false, err
	}
	if r.log != nil {
		r.fillLogFields(start, "get", 0).WithField("Key", key).Info("[ORM][REDIS][GET]")
	}
	r.logOld(key, "GET", time.Since(start).Microseconds(), 0)
	return val, true, nil
}

func (r *RedisCache) LRange(key string, start, stop int64) ([]string, error) {
	s := time.Now()
	val, err := r.client.LRange(key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	if r.log != nil {
		r.fillLogFields(s, "lrange", -1).WithField("Key", key).
			WithField("start", start).WithField("stop", stop).Info("[ORM][REDIS][LRANGE]")
	}
	r.logOld(key, fmt.Sprintf("LRANGE %d %d", start, stop), time.Since(s).Microseconds(), 0)
	return val, nil
}

func (r *RedisCache) HMget(key string, fields ...string) (map[string]interface{}, error) {
	start := time.Now()
	val, err := r.client.HMGet(key, fields...).Result()
	if err != nil {
		return nil, err
	}
	results := make(map[string]interface{}, len(fields))
	misses := 0
	for index, v := range val {
		if v == nil {
			misses++
		}
		results[fields[index]] = v
	}
	if r.log != nil {
		r.fillLogFields(start, "hmget", misses).WithField("Key", key).
			WithField("fields", fields).Info("[ORM][REDIS][HMGET]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("HMGET %v", fields), time.Since(start).Microseconds(), misses)
	}
	return results, nil
}

func (r *RedisCache) HGetAll(key string) (map[string]string, error) {
	start := time.Now()
	val, err := r.client.HGetAll(key).Result()
	if err != nil {
		return nil, err
	}
	if r.log != nil {
		r.fillLogFields(start, "hgetall", -1).WithField("Key", key).Info("[ORM][REDIS][HGETALL]")
	}
	if r.loggers != nil {
		r.logOld(key, "HGETALL", time.Since(start).Microseconds(), 0)
	}
	return val, nil
}

func (r *RedisCache) LPush(key string, values ...interface{}) (int64, error) {
	start := time.Now()
	val, err := r.client.LPush(key, values...).Result()
	if err != nil {
		return 0, err
	}
	if r.log != nil {
		r.fillLogFields(start, "lpush", -1).
			WithField("Key", key).WithField("values", values).Info("[ORM][REDIS][LPUSH]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("LPUSH %d values", len(values)), time.Since(start).Microseconds(), 0)
	}
	return val, nil
}

func (r *RedisCache) RPush(key string, values ...interface{}) (int64, error) {
	start := time.Now()
	val, err := r.client.RPush(key, values...).Result()
	if err != nil {
		return 0, err
	}
	if r.log != nil {
		r.fillLogFields(start, "rpush", -1).
			WithField("Key", key).WithField("values", values).Info("[ORM][REDIS][RPUSH]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("RPUSH %d values", len(values)), time.Since(start).Microseconds(), 0)
	}
	return val, nil
}

func (r *RedisCache) RPop(key string) (value string, found bool, err error) {
	start := time.Now()
	val, err := r.client.RPop(key).Result()
	if err != nil {
		if err == redis.Nil {
			if r.log != nil {
				r.fillLogFields(start, "rpop", 1).WithField("Key", key).Info("[ORM][REDIS][RPOP]")
			}
			r.logOld(key, "RPOP", time.Since(start).Microseconds(), 1)
			return "", false, nil
		}
		return "", false, err
	}
	if r.log != nil {
		r.fillLogFields(start, "rpop", 0).WithField("Key", key).Info("[ORM][REDIS][RPOP]")
	}
	r.logOld(key, "RPOP", time.Since(start).Microseconds(), 0)
	return val, true, nil
}

func (r *RedisCache) LSet(key string, index int64, value interface{}) error {
	start := time.Now()
	_, err := r.client.LSet(key, index, value).Result()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "lset", -1).
			WithField("Key", key).WithField("index", index).WithField("value", value).Info("[ORM][REDIS][LSET]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("LSET %d %s", index, value), time.Since(start).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) LRem(key string, count int64, value interface{}) error {
	start := time.Now()
	_, err := r.client.LRem(key, count, value).Result()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "lrem", -1).
			WithField("Key", key).WithField("count", count).WithField("value", value).Info("[ORM][REDIS][LREM]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("LREM %d %s", count, value), time.Since(start).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) Ltrim(key string, start, stop int64) error {
	s := time.Now()
	_, err := r.client.LTrim(key, start, stop).Result()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(s, "ptrim", -1).
			WithField("Key", key).WithField("start", start).WithField("stop", stop).Info("[ORM][REDIS][LTRIM]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("LTRIM %d %d", start, stop), time.Since(s).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) ZCard(key string) (int64, error) {
	start := time.Now()
	val, err := r.client.ZCard(key).Result()
	if r.log != nil {
		r.fillLogFields(start, "zcard", -1).
			WithField("Key", key).Info("[ORM][REDIS][ZCARD]")
	}
	r.logOld(key, "ZCARD", time.Since(start).Microseconds(), 0)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) SCard(key string) (int64, error) {
	start := time.Now()
	val, err := r.client.SCard(key).Result()
	if r.log != nil {
		r.fillLogFields(start, "scard", -1).
			WithField("Key", key).Info("[ORM][REDIS][SCARD]")
	}
	r.logOld(key, "SCARD", time.Since(start).Microseconds(), 0)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) ZCount(key string, min, max string) (int64, error) {
	start := time.Now()
	val, err := r.client.ZCount(key, min, max).Result()
	if r.log != nil {
		r.fillLogFields(start, "zcount", -1).
			WithField("Key", key).WithField("min", min).WithField("max", max).Info("[ORM][REDIS][ZCOUNT]")
	}
	r.logOld(key, "ZCOUNT", time.Since(start).Microseconds(), 0)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) SPop(key string) (string, bool, error) {
	start := time.Now()
	val, err := r.client.SPop(key).Result()
	if r.log != nil {
		r.fillLogFields(start, "spop", -1).
			WithField("Key", key).Info("[ORM][REDIS][SPOP]")
	}
	if r.loggers != nil {
		r.logOld(key, "SPOP", time.Since(start).Microseconds(), 0)
	}
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

func (r *RedisCache) SPopN(key string, max int64) ([]string, error) {
	start := time.Now()
	val, err := r.client.SPopN(key, max).Result()
	if r.log != nil {
		r.fillLogFields(start, "spopn", -1).
			WithField("Key", key).WithField("max", max).Info("[ORM][REDIS][SPOPN]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("SPOP %d", max), time.Since(start).Microseconds(), 0)
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (r *RedisCache) LLen(key string) (int64, error) {
	start := time.Now()
	val, err := r.client.LLen(key).Result()
	if r.log != nil {
		r.fillLogFields(start, "llen", -1).
			WithField("Key", key).Info("[ORM][REDIS][LLEN]")
	}
	r.logOld(key, "LLEN", time.Since(start).Microseconds(), 0)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (r *RedisCache) ZAdd(key string, members ...*redis.Z) (int64, error) {
	start := time.Now()
	val, err := r.client.ZAdd(key, members...).Result()
	if err != nil {
		return 0, err
	}
	if r.log != nil {
		r.fillLogFields(start, "zadd", -1).
			WithField("Key", key).WithField("members", len(members)).Info("[ORM][REDIS][ZADD]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("ZADD %d values", len(members)), time.Since(start).Microseconds(), 0)
	}
	return val, nil
}

func (r *RedisCache) SAdd(key string, members ...interface{}) (int64, error) {
	start := time.Now()
	val, err := r.client.SAdd(key, members...).Result()
	if err != nil {
		return 0, err
	}
	if r.log != nil {
		r.fillLogFields(start, "sadd", -1).
			WithField("Key", key).WithField("members", len(members)).Info("[ORM][REDIS][SADD]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("SADD %d values", len(members)), time.Since(start).Microseconds(), 0)
	}
	return val, nil
}

func (r *RedisCache) HMset(key string, fields map[string]interface{}) error {
	start := time.Now()
	_, err := r.client.HMSet(key, fields).Result()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "hmset", -1).
			WithField("Key", key).WithField("fields", fields).Info("[ORM][REDIS][HMSET]")
	}
	if r.loggers != nil {
		keys := make([]string, len(fields))
		i := 0
		for key := range fields {
			keys[i] = key
			i++
		}
		r.logOld(key, fmt.Sprintf("HMSET %v", keys), time.Since(start).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) HSet(key string, field string, value interface{}) error {
	start := time.Now()
	_, err := r.client.HSet(key, field, value).Result()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "hset", -1).
			WithField("Key", key).WithField("field", field).WithField("value", value).Info("[ORM][REDIS][HSET]")
	}
	if r.loggers != nil {
		r.logOld(key, fmt.Sprintf("HSET %s %s", key, value), time.Since(start).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) MGet(keys ...string) (map[string]interface{}, error) {
	start := time.Now()
	val, err := r.client.MGet(keys...).Result()
	if err != nil {
		return nil, err
	}
	results := make(map[string]interface{}, len(keys))
	misses := 0
	for index, v := range val {
		results[keys[index]] = v
		if v == nil {
			misses++
		}
	}
	if r.log != nil {
		r.fillLogFields(start, "mget", misses).
			WithField("Keys", keys).Info("[ORM][REDIS][MGET]")
	}
	if r.loggers != nil {
		r.logOld(strings.Join(keys, ","), "MGET", time.Since(start).Microseconds(), misses)
	}
	return results, nil
}

func (r *RedisCache) Set(key string, value interface{}, ttlSeconds int) error {
	start := time.Now()
	err := r.client.Set(key, value, time.Duration(ttlSeconds)*time.Second).Err()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "set", -1).
			WithField("Key", key).WithField("value", value).WithField("ttl", ttlSeconds).Info("[ORM][REDIS][SET]")
	}
	r.logOld(key, fmt.Sprintf("SET [%ds]", ttlSeconds), time.Since(start).Microseconds(), 0)
	return nil
}

func (r *RedisCache) MSet(pairs ...interface{}) error {
	start := time.Now()
	err := r.client.MSet(pairs...).Err()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "mset", -1).
			WithField("Pairs", pairs).Info("[ORM][REDIS][MSET]")
	}
	if r.loggers != nil {
		max := len(pairs)
		keys := make([]string, max)
		for i := 0; i < max; i += 2 {
			keys[i] = pairs[i].(string)
		}
		r.logOld("", fmt.Sprintf("MSET %v", keys), time.Since(start).Microseconds(), 0)
	}
	return nil
}

func (r *RedisCache) Del(keys ...string) error {
	start := time.Now()
	err := r.client.Del(keys...).Err()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "del", -1).
			WithField("Keys", keys).Info("[ORM][REDIS][DEL]")
	}
	r.logOld(strings.Join(keys, ","), "DELETE", time.Since(start).Microseconds(), 0)
	return nil
}

func (r *RedisCache) FlushDB() error {
	start := time.Now()
	err := r.client.FlushDB().Err()
	if err != nil {
		return err
	}
	if r.log != nil {
		r.fillLogFields(start, "flushdb", -1).Info("[ORM][REDIS][FLUSHDB]")
	}
	r.logOld("", "FLUSHDB", time.Since(start).Microseconds(), 0)
	return nil
}

func (r *RedisCache) RegisterLogger(logger CacheLogger) {
	if r.loggers == nil {
		r.loggers = make([]CacheLogger, 0)
	}
	r.loggers = append(r.loggers, logger)
}

func (r *RedisCache) logOld(key string, operation string, microseconds int64, misses int) {
	if r.loggers != nil {
		for _, logger := range r.loggers {
			logger.Log("REDIS", r.code, key, operation, microseconds, misses)
		}
	}
}

func (r *RedisCache) fillLogFields(start time.Time, operation string, misses int) *log.Entry {
	e := r.log.
		WithField("microseconds", time.Since(start).Microseconds()).
		WithField("operation", operation).
		WithField("pool", r.code).
		WithField("target", "redis").
		WithField("time", start.Unix())
	if misses >= 0 {
		e = e.WithField("misses", misses)
	}
	return e
}