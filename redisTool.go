package RLLOCK

import (
	"github.com/gomodule/redigo/redis"
	"time"
)

// redis config
type RedisConf struct {
	Host string
	Pass string
	DB   int
	// 连接池中最大空闲连接数
	MaxIdleConnections int
	//给定时间池中分配的最大连接数。
	MaxConnections int
	// 闲置链接超时时间
	IdleTimeout int
}

// redis配置
func CreateRedisConfig(host string, DB int, pass string, idleTimeout int, maxConnections int, maxIdleConnections int) (redisConf RedisConf) {
	return RedisConf{
		Host:               host,
		Pass:               pass,
		DB:                 DB,
		IdleTimeout:        idleTimeout,
		MaxConnections:     maxConnections,
		MaxIdleConnections: maxIdleConnections,
	}
}

// 创建连接池
func ConnectRedisPool(redisConf RedisConf) (pool *redis.Pool, err error) {

	pool = &redis.Pool{
		MaxIdle:     redisConf.MaxIdleConnections,
		MaxActive:   redisConf.MaxConnections,
		IdleTimeout: time.Duration(redisConf.IdleTimeout) * time.Second,
		Dial: func() (redis.Conn, error) {
			timeout := 500 * time.Millisecond
			c, err := redis.Dial("tcp", redisConf.Host, redis.DialPassword(redisConf.Pass), redis.DialDatabase(redisConf.DB), redis.DialConnectTimeout(timeout),
				redis.DialReadTimeout(timeout), redis.DialWriteTimeout(timeout))
			if err != nil {
				return c, err
			}
			return c, nil
		},
	}

	return
}
