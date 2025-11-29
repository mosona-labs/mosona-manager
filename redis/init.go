package redis

import (
	"fmt"
	"mosona-manager/config"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func Init() {
	Client = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", config.Conf.RedisHost, config.Conf.RedisPort),
		Password:     config.Conf.RedisPassword,
		PoolSize:     100,
		MinIdleConns: 10,
	})
}
