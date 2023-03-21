package cache_redis

import (
	"github.com/infrago/cache"
	"github.com/infrago/infra"
)

func Driver() cache.Driver {
	return &redisDriver{}
}

func init() {
	infra.Register("redis", Driver())
}
