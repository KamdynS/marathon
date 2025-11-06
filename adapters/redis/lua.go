package redisstore

// luaAppendEvent atomically increments per-workflow sequence, embeds it into the
// event JSON, appends to the workflow ZSET (score=seq), and updates the
// workflow state's last_event_seq field if present.
//
// KEYS[1] = seq key
// KEYS[2] = events zset key
// KEYS[3] = workflow state key (JSON string)
// ARGV[1] = event JSON string
//
// Returns: sequence (number)
const luaAppendEvent = `
local seq = redis.call('INCR', KEYS[1])

-- prepare event JSON with sequence_num set
local ev = cjson.decode(ARGV[1])
ev['sequence_num'] = seq
local evjson = cjson.encode(ev)

-- append to events zset with score=seq
redis.call('ZADD', KEYS[2], seq, evjson)

-- update workflow state's last_event_seq if present
local stjson = redis.call('GET', KEYS[3])
if stjson then
  local st = cjson.decode(stjson)
  st['last_event_seq'] = seq
  redis.call('SET', KEYS[3], cjson.encode(st))
end

return seq
`

// luaMarkTimerFired atomically marks a timer as fired if not already, and
// removes it from the due set. Returns 1 if transitioned, 0 otherwise.
//
// KEYS[1] = timer record key (JSON string)
// KEYS[2] = timers due zset key
// ARGV[1] = due member (workflowID:timerID)
const luaMarkTimerFired = `
local recjson = redis.call('GET', KEYS[1])
if not recjson then
  return 0
end
local rec = cjson.decode(recjson)
if rec['fired'] == true then
  return 0
end
rec['fired'] = true
redis.call('SET', KEYS[1], cjson.encode(rec))
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`


