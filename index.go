package cache_redis

import (
	"github.com/infrago/cache"
)

func Driver() cache.Driver {
	return &redisDriver{}
}

func init() {
	cache.Register("redis", Driver())
}
