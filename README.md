# orm

![Check & test](https://github.com/summer-solutions/orm/workflows/Check%20&%20test/badge.svg)
[![codecov](https://codecov.io/gh/summer-solutions/orm/branch/master/graph/badge.svg)](https://codecov.io/gh/summer-solutions/orm)
[![Go Report Card](https://goreportcard.com/badge/github.com/summer-solutions/orm)](https://goreportcard.com/report/github.com/summer-solutions/orm)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)



ORM that delivers support for full stack data access:

 * MySQL - for relational data
 * Redis - for NoSQL in memory shared cache
 * Elastic Search - for full text search
 * Local Cache - in memory local (not shared) cache
 * ClickHouse - time series database
 * RabbitMQ - message broker 

## Configuration

First you need to define Registry object and register all connection pools to MySQL, Redis, RabbitMQ and local cache.
Use this object to register queues, and entities. You should create this object once when application
starts.

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    registry := &Registry{}

    /*MySQL */
    registry.RegisterMySQLPool("root:root@tcp(localhost:3306)/database_name")
    //optionally you can define pool name as second argument
    registry.RegisterMySQLPool("root:root@tcp(localhost:3307)/database_name", "second_pool")

    /* Redis */
    registry.RegisterRedis("localhost:6379", 0)
    //optionally you can define pool name as second argument
    registry.RegisterRedis("localhost:6379", 1, "second_pool")

    /* RabbitMQ */
    registry.RegisterRabbitMQServer("amqp://rabbitmq_user:rabbitmq_password@localhost:5672/")
    registry.RegisterRabbitMQQueue(&RabbitMQQueueConfig{Name: "test_queue"})
    registry.RegisterRabbitMQRouter(&RabbitMQRouterConfig{Name, "test_router"})

    /* Local cache (in memory) */
    registry.RegisterLocalCache(1000) //you need to define cache size
    //optionally you can define pool name as second argument
    registry.RegisterLocalCache(100, "second_pool")

    /* Redis used to handle locks (explained later) */
    registry.RegisterRedis("localhost:6379", 4, "lockers_pool")
    registry.RegisterLocker("default", "lockers_pool")
}

```

##### You can also create registry using yaml configuration file:

```.yaml
default:
    mysql: root:root@tcp(localhost:3310)/db
    redis: localhost:6379:0
    locker: default
    dirtyQueues:
        test: 10
        test2: 1    
    localCache: 1000
    rabbitMQ:
        server: amqp://rabbitmq_user:rabbitmq_password@localhost:5672/
        queues:
            - name: test
            - name: test2
              durrable: false // optional, default true
              autodelete: false // optional, default false
              prefetchCount: 1 // optional, default 1
              router: test // optional, default ""
              router_keys: // optional, default []string
                - aa
                - bb
        routers:
            - name: test_delayed
              type: direct  
              delayed: true //optional, default false  
              durable: false // optional, default true
second_pool:
    mysql: root:root@tcp(localhost:3311)/db2
    redis: localhost:6380:1 
```

```go
package main

import (
    "github.com/summer-solutions/orm"
    "gopkg.in/yaml.v2"
    "io/ioutil"
)

func main() {

    yamlFileData, err := ioutil.ReadFile("./yaml")
    if err != nil {
        //...
    }
    
    var parsedYaml map[string]interface{}
    err = yaml.Unmarshal(yamlFileData, &parsedYaml)
    registry, err := InitByYaml(parsedYaml)
}

```

## Defining entities

```go
package main

import (
	"github.com/summer-solutions/orm"
	"time"
)

func main() {

    type AddressSchema struct {
        Street   string
        Building uint16
    }
    
    type colors struct {
        Red    string
        Green  string
        Blue   string
        Yellow string
        Purple string
    }
    var Colors = &colors{
        orm.EnumModel,
    	Red:    "Red",
    	Green:  "Green",
    	Blue:   "Blue",
    	Yellow: "Yellow",
    	Purple: "Purple",
    }

    type testEntitySchema struct {
        orm.ORM
        ID                   uint
        Name                 string `orm:"length=100;index=FirstIndex"`
        NameNotNull          string `orm:"length=100;index=FirstIndex;required"`
        BigName              string `orm:"length=max"`
        Uint8                uint8  `orm:"unique=SecondIndex:2,ThirdIndex"`
        Uint24               uint32 `orm:"mediumint=true"`
        Uint32               uint32
        Uint64               uint64 `orm:"unique=SecondIndex"`
        Int8                 int8
        Int16                int16
        Int32                int32
        Int64                int64
        Rune                 rune
        Int                  int
        Bool                 bool
        Float32              float32
        Float64              float64
        Float32Decimal       float32  `orm:"decimal=8,2"`
        Float64DecimalSigned float64  `orm:"decimal=8,2;unsigned=false"`
        Enum                 string   `orm:"enum=orm.colorEnum"`
        EnumNotNull          string   `orm:"enum=orm.colorEnum;required"`
        Set                  []string `orm:"set=orm.colorEnum"`
        Year                 uint16   `orm:"year=true"`
        YearNotNull          uint16   `orm:"year=true;required"`
        Date                 *time.Time
        DateNotNull          time.Time
        DateTime             *time.Time `orm:"time=true"`
        DateTimeNotNull      time.Time  `orm:"time=true"`
        Address              AddressSchema
        Json                 interface{}
        ReferenceOne         *testEntitySchemaRef
        ReferenceOneCascade  *testEntitySchemaRef `orm:"cascade"`
        IgnoreField          []time.Time       `orm:"ignore"`
        Blob                 []byte
    }
    
    type testEntitySchemaRef struct {
        orm.ORM
        ID   uint
        Name string
    }
    type testEntitySecondPool struct {
    	orm.ORM `orm:"mysql=second_pool"`
    	ID                   uint
    }

    registry := &Registry{}
    var testEntitySchema testEntitySchema
    var testEntitySchemaRef testEntitySchemaRef
    var testEntitySecondPool testEntitySecondPool
    registry.RegisterEntity(testEntitySchema, testEntitySchemaRef, testEntitySecondPool)
    registry.RegisterEnumStruct("color", Colors)

    // now u can use:
    Colors.GetDefault() // "Red" (first field)
    Colors.GetFields() // ["Red", "Blue" ...]
    Colors.GetMapping() // map[string]string{"Red": "Red", "Blue": "Blue"}
    Colors.Has("Red") //true
    Colors.Has("Orange") //false
    
    //or register enum from slice
    registry.RegisterEnumSlice("color", []string{"Red", "Blue"})
    validatedRegistry.GetEnum("color").GetFields()
    validatedRegistry.GetEnum("color").Has("Red")
    
    //or register enum from map
    registry.RegisterEnumMap("color", map[string]string{"red": "Red", "blue": "Blue"}, "red")
}
```

There are only two golden rules you need to remember defining entity struct: 

 * first field must be type of "ORM"
 * second argument must have name "ID" and must be type of one of uint, uint16, uint32, uint24, uint64, rune
 
 
 By default entity is not cached in local cache or redis, to change that simply use key "redisCache" or "localCache"
 in "orm" tag for "ORM" field:
 
 ```go
 package main
 
 import (
 	"github.com/summer-solutions/orm"
 	"time"
 )
 
 func main() {
 
     type testEntityLocalCache struct {
     	orm.ORM `orm:"localCache"` //default pool
        //...
     }
    
    type testEntityLocalCacheSecondPool struct {
     	orm.ORM `orm:"localCache=second_pool"`
        //...
     }
    
    type testEntityRedisCache struct {
     	orm.ORM `orm:"redisCache"` //default pool
        //...
     }
    
    type testEntityRedisCacheSecondPool struct {
     	orm.ORM `orm:"redisCache=second_pool"`
        //...
     }

    type testEntityLocalAndRedisCache struct {
     	orm.ORM `orm:"localCache;redisCache"`
        //...
     }
 }
 ```

## Validated registry

Once you created your registry and registered all pools and entities you should validate it.
You should also run it once when your application starts.

 ```go
 package main
 
 import "github.com/summer-solutions/orm"
 
 func main() {
    registry := &Registry{}
    //register pools and entities
    validatedRegistry, err := registry.Validate()
 }
 
 ```
 
 ## Creating engine
 
 You need to crete engine to start working with entities (searching, saving).
 You must create engine for each http request and thread.
 
  ```go
  package main
  
  import "github.com/summer-solutions/orm"
  
  func main() {
     registry := &Registry{}
     //register pools and entities
     validatedRegistry, err := registry.Validate()
     engine := validatedRegistry.CreateEngine()
  }
  
  ```
 
 ## Checking and updating table schema
 
 ORM provides useful object that describes entity structrure called TabelSchema:
 
 ```go
 package main
 
 import "github.com/summer-solutions/orm"
 
 func main() {
    
    registry := &Registry{}
    // register
    validatedRegistry, err := registry.Validate() 
    engine := validatatedRegistry.CreateEngine()
    alters, err := engine.GetAlters()
    
    /*optionally you can execute alters for each model*/
    var userEntity UserEntity
    tableSchema := engine.GetRegistry().GetTableSchemaForEntity(userEntity)
    //or
    tableSchema := validatedRegistry.GetTableSchemaForEntity(userEntity)

    /*checking table structure*/
    tableSchema.UpdateSchema(engine) //it will create or alter table if needed
    tableSchema.DropTable(engine) //it will drop table if exist
    tableSchema.TruncateTable(engine)
    tableSchema.UpdateSchemaAndTruncateTable(engine)
    has, alters, err := tableSchema.GetSchemaChanges(engine)

    /*getting table structure*/
    db := tableSchema.GetMysql(engine)
    localCache, has := tableSchema.GetLocalCache(engine) 
    redisCache, has := tableSchema.GetRedisCache(engine)
    columns := tableSchema.GetColumns()
    tableSchema.GetTableName()
 }
 
 ```


## Adding, editing, deleting entities

```go
package main

import "github.com/summer-solutions/orm"

func main() {

     /* adding */

    entity := testEntity{Name: "Name 1"}
    err := engine.TrackAndFlush(&entity)

    entity2 := testEntity{Name: "Name 1"}
    engine.SetOnDuplicateKeyUpdate(NewWhere("`counter` = `counter` + 1"))
    err := engine.TrackAndFlush(&entity)

    entity2 = testEntity{Name: "Name 1"}
    engine.SetOnDuplicateKeyUpdate(NewWhere("")) //it will chnage nothing un row
    err := engine.TrackAndFlush(&entity)

    /*if you need to add more than one entity*/
    entity = testEntity{Name: "Name 2"}
    entity2 := testEntity{Name: "Name 3"}
    engine.Track(&entity, &entity2) //it will also automatically run RegisterEntity()
    //it will execute only one query in MySQL adding two rows at once (atomic)
    err = engine.Flush()
 
    /* editing */

    engine.Track(&entity, &entity2)
    entity.Name = "New name 2"
    //you can also use (but it's slower):
    err = entity.SetField("Name", "New name 2")
    engine.IsDirty(entity) //returns true
    engine.IsDirty(entity2) //returns false
    err = entity.Flush() //it will save data in DB for all dirty tracked entities and untrack all of them
    engine.IsDirty(entity) //returns false
    
    /* deleting */
    engine.MarkToDelete(entity2)
    engine.IsDirty(entity2) //returns true
    err = engine.Flush()

    /* flush can return 2 special errors */
    DuplicatedKeyError{} //when unique index is broken
    ForeignKeyError{} //when foreign key is broken
}
```

## Transactions

```go
package main

import "github.com/summer-solutions/orm"

func main() {
	
    entity = testEntity{Name: "Name 2"}
    entity2 := testEntity{Name: "Name 3"}
    engine.Track(&entity, &entity2)

    // DB transcation
    err = engine.FlushInTransaction()
    // or redis lock
    err = engine.FlushWithLock("default", "lock_name", 10 * time.Second, 10 * time.Second)
    // or DB transcation nad redis lock
    err = engine.FlushInTransactionWithLock("default", "lock_name", 10 * time.Second, 10 * time.Second)
 
    //manual transaction
    db := engine.GetMysql()
    db.Begin()
    defer db.Rollback()
    //run queries
    db.Commit()
```

## Loading entities using primary key

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    var entity testEntity
    has, err := engine.LoadByID(1, &entity)

    var entities []*testEntity
    missing, err := engine.LoadByIDs([]uint64{1, 3, 4}, &entities) //missing contains IDs that are missing in database

}

```

## Loading entities using search

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    var entities []*testEntity
    pager := Pager{CurrentPage: 1, PageSize: 100}
    where := NewWhere("`ID` > ? AND `ID` < ?", 1, 8)
    err := engine.Search(where, pager, &entities)
    
    //or if you need number of total rows
    totalRows, err := engine.SearchWithCount(where, pager, &entities)
    
    //or if you need only one row
    where := NewWhere("`Name` = ?", "Hello")
    var entity testEntity
    found, err := engine.SearchOne(where, &entity)
    
    //or if you need only primary keys
    ids, err := engine.SearchIDs(where, pager, entity)
    
    //or if you need only primary keys and total rows
    ids, totalRows, err = engine.SearchIDsWithCount(where, pager, entity)
}

```

## Reference one to one

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    type UserEntity struct {
        ORM
        ID                   uint64
        Name                 string
        School               *SchoolEntity `orm:"required"` // key is "on delete restrict" by default not not nullable
        SecondarySchool      *SchoolEntity // key is nullable
    }
    
    type SchoolEntity struct {
        ORM
        ID                   uint64
        Name                 string
    }

    type UserHouse struct {
        ORM
        ID                   uint64
        User                 *UserEntity  `orm:"cascade;required"` // on delete cascade and is not nullable
    }
    
    // saving in DB:

    user := UserEntity{Name: "John"}
    school := SchoolEntity{Name: "Name of school"}
    house := UserHouse{Name: "Name of school"}
    engine.Track(&user, &school, &house)
    user.School = school
    house.User = user
    engine.Flush()

    // loading references: 

    _, err = engine.LoadById(1, &user)
    user.School != nil //returns true, School has ID: 1 but other fields are nof filled
    user.School.ID == 1 //true
    user.School.Loaded() //false
    user.Name == "" //true
    err = user.School.Load(engine) //it will load school from db
    user.School.Loaded() //now it's true, you can access school fields like user.School.Name
    user.Name == "Name of school" //true
    
    //If you want to set reference and you have only ID:
    user.School = &SchoolEntity{ID: 1}

    // detaching reference
    user.School = nil

    // preloading references
    engine.LoadByID(1, &user, "*") //all references
    engine.LoadByID(1, &user, "School") //only School
    engine.LoadByID(1, &user, "School", "SecondarySchool") //only School and SecondarySchool
    engine.LoadByID(1, &userHouse, "User/School", "User/SecondarySchool") //User, School and SecondarySchool in each User
    engine.LoadByID(1, &userHouse, "User/*") // User, all references in User
    engine.LoadByID(1, &userHouse, "User/*/*") // User, all references in User and all references in User subreferences
    //You can have as many levels you want: User/School/AnotherReference/EvenMore/
    
    //You can preload referenes in all search and load methods:
    engine.LoadByIDs()
    engine.Search()
    engine.SearchOne()
    engine.CachedSearch()
    ...
}

```

## Cached queries

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    //Fields that needs to be tracked for changes should start with ":"

    type UserEntity struct {
        ORM
        ID                   uint64
        Name                 string
        Age                  uint16
        IndexAge             *CachedQuery `query:":Age = ? ORDER BY :ID"`
        IndexAll             *CachedQuery `query:""` //cache all rows
        IndexName            *CachedQuery `queryOne:":Name = ?" orm:"max=100"` // be default cached query can cache max 50 000 rows
    }

    pager := &Pager{CurrentPage: 1, PageSize: 100}
    var users []*UserEntity
    var user  UserEntity
    totalRows, err := engine.CachedSearch(&users, "IndexAge", pager, 18)
    totalRows, err = engine.CachedSearch(&users, "IndexAll", pager)
    has, err := engine.CachedSearchOne(&user, "IndexName", "John")

}

```

## Lazy flush

Sometimes you want to flush changes in database, but it's ok if data is flushed after some time. 
For example when you want to save some logs in database.

```go
package main

import "github.com/summer-solutions/orm"

func main() {
    
    // you need to register default rabbitMQ server    
    registry.RegisterRabbitMQServer("amqp://rabbitmq_user:rabbitmq_password@localhost:5672/")
    
    // now in code you can use FlushLazy() methods instead of Flush().
    // it will send changes to queue (database and cached is not updated yet)
    user.FlushLazy()
    
    //You need to run code that will read data from queue and execute changes
    
    receiver := NewLazyReceiver(engine)
    //optionally 
    err := receiver.Digest() //It will wait for new messages in queue, run receiver.DisableLoop() to run loop once
}

```

## Log entity changes

ORM can store in database every change of entity in special log table.

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    //it's recommended to keep logs in separated DB
    registry.RegisterMySQLPool("root:root@tcp(localhost:3306)/log_database", "log_db_pool")
    // you need to register default rabbitMQ server    
    registry.RegisterRabbitMQServer("amqp://rabbitmq_user:rabbitmq_password@localhost:5672/")

    //next you need to define in Entity that you want to log changes. Just add "log" tag
    type User struct {
        ORM  `orm:"log=log_db_pool"`
        ID   uint
        Name string
        Age  int
    }

    // Now every change of User will be saved in log table
    
    // You can add extra data to log, simply use this methods before Flush():
    engine.SetLogMetaData("logged_user_id", 12) 
    engine.SetLogMetaData("ip", request.GetUserIP())
    // you can set meta only in specific entity
    engine.SetEntityLogMeta("user_name", "john", entity)
    
    receiver := NewLogReceiver(engine)
    err := receiver.Digets() //it will wait for new messages in queue
}

```

## Dirty queues

You can send event to queue if any specific data in entity was changed.

```go
package main

import "github.com/summer-solutions/orm"

func main() {
    
    registry.RegisterRabbitMQServer("amqp://rabbitmq_user:rabbitmq_password@localhost:5672/")
    // register dirty queue
    registry.RegisterDirtyQueue("user_changed", 100)
    registry.RegisterDirtyQueue("age_name_changed", 100)
    registry.RegisterDirtyQueue("age_changed", 100)

    // next you need to define in Entity that you want to log changes. Just add "log" tag
    type User struct {
        orm.ORM  `orm:"dirty=user_changed"` //define dirty here to track all changes
        ID       uint
        Name     string `orm:"dirty=age_name_changed"` //event will be send to age_name_changed if Name or Age changed
        Age      int `orm:"dirty=age_name_changed,age_changed"` //event will be send to age_changed if Age changed
    }

    // now just use Flush and events will be send to queue

    // receiving events
    receiver := NewDirtyReceiver(engine)
    
    // in this case data length is max 100
    err := receiver.Digest("user_changed", func(data []*DirtyData) error {
        for _, item := range data {
            // data.TableSchema is TableSchema of entity
            // data.ID has entity ID
            // data.Added is true if entity was added
            // data.Updated is true if entity was updated
            // data.Deleted is true if entity was deleted
        }
        return nil
    })
}


```

## Set defaults

If you need to define default values for entity simply extend DefaultValuesInterface.

```go
func main() {

    type UserEntity struct {
        ORM
        ID                   uint64
        Name                 string
    }

    func (e *UserEntity) SetDefaults() {
        e.Name = "Tom"
    }
    
}

```

## Fake delete

If you want to keep deleted entity in database but ny default this entity should be excluded
from all engine.Search() and engine.CacheSearch() queries you can use FakeDelete column. Simply create
field bool with name "FakeDelete".

```go
func main() {

    type UserEntity struct {
        ORM
        ID                   uint64
        Name                 string
        FakeDelete           bool
    }

    //you can delete in two ways:
    engine.MarkToDelete(user) -> will set user.FakeDelete = true
    //or:
    user.FakeDelete = true

    engine.Flush(user) //it will save entity id in Column `FakeDelete`.

    //will return all rows where `FakeDelete` = 0
    total, err = engine.SearchWithCount(NewWhere("1"), nil, &rows)

    //To force delete (remove row from DB):
    engine.ForceMarkToDelete(user)
    engine.Flush(user)
}


```

## After saved

If you need to execute code after entity is added or updated simply extend AfterSavedInterface.

```go
func main() {

    type UserEntity struct {
        ORM
        ID                   uint64
        Value                int
        Calculated           string `orm:"ignore"`
    }

    func (e *UserEntity) AfterSaved(engine *Engine) error {
        e.Calculated = e.Value + 1
        return nil
    }
}

```

## Working with Redis

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    config.RegisterRedis("localhost:6379", 0)
    
    //storing data in cached for x seconds
    val, err := engine.GetRedis().GetSet("key", 1, func() interface{} {
		return "hello"
	})
    
    //standard redis api
    keys, err := engine.GetRedis().LRange("key", 1, 2)
    err = engine.GetRedis().LPush("key", "a", "b")
    //...
}

```


## Working with local cache

```go
package main

import "github.com/summer-solutions/orm"

func main() {
    
    registry.RegisterLocalCache(1000)
    
    //storing data in cached for x seconds
    val := engine.GetLocalCache().GetSet("key", 1, func() interface{} {
        return "hello"
    })
    
    //getting value
    value, has := engine.GetLocalCache().Get("key")
    
    //getting many values
    values := engine.GetLocalCache().MGet("key1", "key2")
    
    //setting value
    engine.GetLocalCache().Set("key", "value")
    
    //setting values
    engine.GetLocalCache().MSet("key1", "value1", "key2", "value2" /*...*/)
    
    //getting values from hash set (like redis HMGET)
    values = engine.GetLocalCache().HMget("key")
    
    //setting values in hash set
    engine.GetLocalCache().HMset("key", map[string]interface{}{"key1" : "value1", "key2": "value 2"})

    //deleting value
    engine.GetLocalCache().Remove("key1", "key2" /*...*/)
    
    //clearing cache
    engine.GetLocalCache().Clear()

}

```

## Working with mysql

```go
package main

import (
    "database/sql"
    "github.com/summer-solutions/orm"
)

func main() {
    
    // register mysql pool
    registry.RegisterMySQLPool("root:root@tcp(localhost:3306)/database_name")

    res, err := engine.GetMysql().Exec("UPDATE `table_name` SET `Name` = ? WHERE `ID` = ?", "Hello", 2)

    var row string
    err = engine.GetMysql().QueryRow("SELECT * FROM `table_name` WHERE  `ID` = ?", 1).Scan(&row)
    if err != nil {
        if err != sql.ErrNoRows {
            ///...
        }
        //no row found
    }
    
    results, def, err := engine.GetMysql().Query("SELECT * FROM `table_name` WHERE  `ID` > ? LIMIT 100", 1)
    //handle err
    defer def()
    for results.Next() {
    	var row string
        err = results.Scan(&row)
    }
    err = results.Err()
    //handle err

}

```

## Working with Locker

Shared cached that is using redis

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    // register redis and locker
    registry.RegisterRedis("localhost:6379", 0, "my_pool")
    registry.RegisterLocker("default", "my_pool")
    
    locker, _ := engine.GetLocker()
    lock, err := locker.Obtain("my_lock", 5 * Time.Second, 1 * Time.Second)
    if err != nil {
        panic(err)
    }
    defer lock.Release()
    
    // do smth
    
    ttl, err := lock.TTL()
    if ttl == 0 {
        panic("lock lost")
    }
}

```

## Working with RabbitMQ

```go
package main

import "github.com/summer-solutions/orm"

func main() {

    // register rabbitMQ servers, queues and routers
    registry.RegisterRabbitMQServer("amqp://rabbitmq_user:rabbitmq_password@localhost:5672/")
    registry.RegisterRabbitMQQueue(&RabbitMQQueueConfig{Name: "test_queue"})
    registry.RegisterRabbitMQQueue(&RabbitMQQueueConfig{Name: "test_queue_delayed", Delayed: true})
    registry.RegisterRabbitMQQueue(&RabbitMQQueueConfig{Name: "test_queue_router", 
        Router: "test_router", RouteKeys: []string{"aa", "bb"}})
    registry.RegisterRabbitMQRoutere("default", &RabbitMQRouteConfig{Name: "test_router", Type: "fanout"})
    
    //create engine:
    validatedRegistry, err := registry.Validate()
    engine := validatedRegistry.CreateEngine()
    defer engine.Defer() //it will close all opened channels

    //Simple queue
    channel := engine.GetRabbitMQQueue("test_queue") //provide Queue name
    defer channel.Close()
    err = channel.Publish([]byte("hello"))

    //start consumer (you can add as many you want)
    consumer, err := channel.NewConsumer("test consumer")
    defer consumer.Close()
    err := consumer.Consume(func(items [][]byte) error {
    	//do staff
    	return nil   
    })

    //Delayed queue
    channel := engine.GetRabbitMQDelayedQueue("test_queue_delayed") //provide Queue name
    defer channel.Close()
    err = channel.Publish([]byte("hello"), time.Minute * 10)

    //start consumer (you can add as many you want)
    consumer, err := channel.NewConsumer("test consumer")
    defer consumer.Close()
    err := consumer.Consume(func(items [][]byte) error {
        //do staff
    	return nil
    })
    
    // publish to router

    channel = engine.GetRabbitMQRouter("test_queue_router") 
    defer channel.Close()
    err = channel.Publish("router.key", []byte("hello"))

    //start consumer
   consumer, err := channel.NewConsumer("test consumer")
   defer consumer.Close()
   err := consumer.Consume(func(items [][]byte) error {
        //do staff
        return nil
   })
}

```


## Logging

Read more: [APEX Log](https://github.com/apex/log)

```go
package main

import "github.com/summer-solutions/orm"

func main() {
	
    //enable human friendly console log
    validatedRegistry.EnableDebug() // for all database, redis, local cache queries in all engines created from this registry
    engine.EnableDebug() //for all database, redis, local cache queries only in this engine
    engine.GetRedis().EnableDebug() //only queries in this redis in this engine
    engine.GetMysql().EnableDebug() //only queries in this DB in this engine
    engine.GetLocalCache().EnableDebug() //only queries in this local cache in this engine

    //adding custom logger example:
    engine.AddLogger(json.New(os.Stdout))
}    
```