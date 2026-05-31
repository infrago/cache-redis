package cache_redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/infrago/cache"
	"github.com/infrago/infra"
	"github.com/redis/go-redis/v9"
)

type redisDriver struct{}

type redisConnection struct {
	client  *redis.Client
	timeout time.Duration
	unlink  bool
}

const sequenceManyScript = `
local count = tonumber(ARGV[4])
if count <= 0 then
	return {}
end
local current
if redis.call("EXISTS", KEYS[1]) == 0 then
	current = tonumber(ARGV[1])
else
	current = tonumber(redis.call("GET", KEYS[1])) + tonumber(ARGV[2])
end
local vals = {}
for i = 1, count do
	if i > 1 then
		current = current + tonumber(ARGV[2])
	end
	vals[i] = current
end
redis.call("SET", KEYS[1], current)
if tonumber(ARGV[3]) > 0 then
	redis.call("PEXPIRE", KEYS[1], ARGV[3])
end
return vals
`

func init() {
	infra.Register("redis", &redisDriver{})
}

func (d *redisDriver) Connect(inst *cache.Instance) (cache.Connect, error) {
	addr, _ := inst.Config.Setting["server"].(string)
	if addr == "" {
		addr, _ = inst.Config.Setting["addr"].(string)
	}
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	username, _ := inst.Config.Setting["username"].(string)
	password, _ := inst.Config.Setting["password"].(string)
	timeout := 3 * time.Second
	if v, ok := durationSetting(inst.Config.Setting["timeout"]); ok && v > 0 {
		timeout = v
	}
	unlink := false
	if v, ok := boolSetting(inst.Config.Setting["unlink"]); ok {
		unlink = v
	}

	db := 0
	if v, ok := intSetting(inst.Config.Setting["database"]); ok {
		db = v
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       db,
	})

	return &redisConnection{client: client, timeout: timeout, unlink: unlink}, nil
}

func (c *redisConnection) Open() error {
	ctx, cancel := c.context()
	defer cancel()
	return c.client.Ping(ctx).Err()
}
func (c *redisConnection) Close() error { return c.client.Close() }

func (c *redisConnection) Read(key string) ([]byte, error) {
	ctx, cancel := c.context()
	defer cancel()
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return val, err
}

func (c *redisConnection) Write(key string, val []byte, expire time.Duration) error {
	ctx, cancel := c.context()
	defer cancel()
	return c.client.Set(ctx, key, val, expire).Err()
}

func (c *redisConnection) Exists(key string) (bool, error) {
	ctx, cancel := c.context()
	defer cancel()
	cnt, err := c.client.Exists(ctx, key).Result()
	return cnt > 0, err
}

func (c *redisConnection) Delete(key string) error {
	ctx, cancel := c.context()
	defer cancel()
	return c.client.Del(ctx, key).Err()
}

func (c *redisConnection) Sequence(key string, start, step int64, expire time.Duration) (int64, error) {
	vals, err := c.SequenceMany(key, start, step, 1, expire)
	if err != nil {
		return -1, err
	}
	if len(vals) == 0 {
		return -1, nil
	}
	return vals[0], nil
}

func (c *redisConnection) SequenceMany(key string, start, step, count int64, expire time.Duration) ([]int64, error) {
	if count <= 0 {
		return []int64{}, nil
	}
	ctx, cancel := c.context()
	defer cancel()
	val, err := c.client.Eval(
		ctx, sequenceManyScript, []string{key},
		start, step, expire.Milliseconds(), count,
	).Slice()
	if err != nil {
		return nil, err
	}
	vals := make([]int64, 0, len(val))
	for _, item := range val {
		switch v := item.(type) {
		case int64:
			vals = append(vals, v)
		case int:
			vals = append(vals, int64(v))
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, err
			}
			vals = append(vals, n)
		default:
			n, err := strconv.ParseInt(fmt.Sprintf("%v", v), 10, 64)
			if err != nil {
				return nil, err
			}
			vals = append(vals, n)
		}
	}
	return vals, nil
}

func (c *redisConnection) Keys(prefix string) ([]string, error) {
	ctx, cancel := c.context()
	defer cancel()
	iter := c.client.Scan(ctx, 0, redisScanPattern(prefix), 1000).Iterator()
	keys := make([]string, 0)
	for iter.Next(ctx) {
		key := iter.Val()
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, iter.Err()
}

func (c *redisConnection) Clear(prefix string) error {
	ctx, cancel := c.context()
	defer cancel()
	iter := c.client.Scan(ctx, 0, redisScanPattern(prefix), 1000).Iterator()
	keys := make([]string, 0, 1000)
	for iter.Next(ctx) {
		key := iter.Val()
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		keys = append(keys, key)
		if len(keys) >= 1000 {
			if err := c.deleteKeys(ctx, keys); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.deleteKeys(ctx, keys)
	}
	return nil
}

func (c *redisConnection) context() (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), c.timeout)
}

func (c *redisConnection) deleteKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	if c.unlink {
		return c.client.Unlink(ctx, keys...).Err()
	}
	return c.client.Del(ctx, keys...).Err()
}

func redisScanPattern(prefix string) string {
	if prefix == "" {
		return "*"
	}
	var b strings.Builder
	b.Grow(len(prefix) + 1)
	for _, r := range prefix {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('*')
	return b.String()
}

func intSetting(v any) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case int8:
		return int(vv), true
	case int16:
		return int(vv), true
	case int32:
		return int(vv), true
	case int64:
		return int(vv), true
	case uint:
		return int(vv), true
	case uint8:
		return int(vv), true
	case uint16:
		return int(vv), true
	case uint32:
		return int(vv), true
	case uint64:
		return int(vv), true
	case float32:
		return int(vv), true
	case float64:
		return int(vv), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(vv))
		return n, err == nil
	default:
		return 0, false
	}
}

func durationSetting(v any) (time.Duration, bool) {
	switch vv := v.(type) {
	case time.Duration:
		return vv, true
	case int:
		return time.Duration(vv) * time.Second, true
	case int64:
		return time.Duration(vv) * time.Second, true
	case float64:
		return time.Duration(vv * float64(time.Second)), true
	case string:
		text := strings.TrimSpace(vv)
		d, err := time.ParseDuration(text)
		if err != nil {
			n, parseErr := strconv.ParseFloat(text, 64)
			if parseErr != nil {
				return 0, false
			}
			return time.Duration(n * float64(time.Second)), true
		}
		return d, err == nil
	default:
		return 0, false
	}
}

func boolSetting(v any) (bool, bool) {
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		switch strings.ToLower(strings.TrimSpace(vv)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		default:
			return false, false
		}
	case int:
		return vv != 0, true
	case int64:
		return vv != 0, true
	default:
		return false, false
	}
}
