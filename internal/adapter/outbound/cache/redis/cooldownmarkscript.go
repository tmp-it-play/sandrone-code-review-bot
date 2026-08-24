package redis

import "github.com/redis/go-redis/v9"

var markCooldownScript = redis.NewScript(`
local current_ttl = redis.call("PTTL", KEYS[1])
local requested_ttl = tonumber(ARGV[2])
if current_ttl >= requested_ttl then
  return 0
end
redis.call("SET", KEYS[1], ARGV[1], "PX", requested_ttl)
return 1
`)
