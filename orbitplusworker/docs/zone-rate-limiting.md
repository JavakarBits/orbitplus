# Zone-wise BITS Rate Limiting (Thanglish Guide)

Idhu orbitplusworker la irukkura **zone-wise rate limiting** eppadi work aagudhu-nu
explain panra document. Simple-a, example oda.

---

## 1. Enna problem-a solve panrom?

Worker queue la irundhu task edukkum. Ovvoru task-ku BITS la irundhu periya data
fetch pannanum. BITS la romba data iruku, adhanaala **oru zone-a adhikam hit
pannaama** control pannanum.

Rule: **Ovvoru zone-ku thani rate limit.** Oru zone limit aana, matha zones
affect aagakoodadhu.

---

## 2. "Zone" na enna?

Zone = oru BITS host. Queue message la `ZoneURL` nu varudhu. Adhu dhaan zone
identifier. (Master service `orionmax_zone_url.go` la zone code -> URL map
pannudhu.)

```
bits    -> http://app.ezeebits.com
r2bits  -> http://app.r2.ezeebits.com
```

Rate limit key ovvoru zone URL ku thani-a maintain aagudhu.

---

## 3. Rate limit model: "hits / window"

Quota format = **hits/window**.

- `10/1m` = oru zone ku 1 nimisham la **max 10 hits**.
- `10/2m` = oru zone ku 2 nimisham la max 10 hits.

Idhu **fixed window counter**. Window **first hit** la start aagudhu (wall-clock
minute la illa).

---

## 4. Zone A 10 hits 1 nimisham la complete aagala na enna?

Example: **Zone A = 10/1m**.

```
10:00:00  Zone A hit 1  -> count=1  -> allowed, key 10:01:00 ku expire aagum
10:00:15  Zone A hit 2  -> count=2  -> allowed
10:00:40  Zone A hit 3  -> count=3  -> allowed
          (indha nimisham la 3 hits mattum, 7 hits use panname illa)
10:01:00  key TTL expire -> counter DELETE aagum (0 ku reset)
10:01:20  Zone A hit     -> count=1 -> allowed, PUDHU window 10:02:20 ku expire
```

**Mukkiyam:**

- Use panna 3 hits ellame allowed. Edhuvum block aagala.
- 1 nimisham mudinjadhum counter **0 ku reset** — use panna illatha 7 hits
  **carry over aagaadhu** (waste aagidum).
- Adutha nimisham fresh 10 hits kidaikum. `10 + 7` illa, **10 mattum**.

Idhu token bucket illa — save panna mudiyaadhu. Fixed window.

### Block eppo aagum?

Window ukkulla count 10-a thaandi 11 aana block aagum.

---

## 5. Block aana task ku enna nadakkudhu? (WAIT — requeue ILLA)

**Mukkiyamaana maatram:** rate-limited task-a queue-ku thirumba pottu (requeue)
panradhu **illa**. Adhukku badhila, athe goroutine **anga-ye wait pannum**, window
reset aana piragu **athe task-a process pannum**.

```
10:00:45  Zone A hit 11 -> Acquire -> BLOCKED (count=11 > 10)
          goroutine wait pannudhu ~15s (10:01:00 varaikum, window reset varaikum)
10:01:00  window reset -> Acquire -> count=1 -> ALLOWED
          athe task (hit 11) ippo process aagum -> BITS call
```

Logic (`awaitZoneRateLimit`):

1. `Acquire(zone)` — allowed na, udane process.
2. Blocked na, `retryAfter` (window reset aaga irukkura exact time) wait pannum.
3. Marubadi `Acquire` — allowed varaikum loop.
4. Context cancel (shutdown) aana mattum error return.

Task **queue la irundhu poga illa**, **drop aaga illa** — athe goroutine kaathirundhu
process pannum.

---

## 6. Zone A um Zone B um separate

**Zone A = 10/1m, Zone B = 10/2m.** Rendukkum thani key, thani count, thani TTL.

```
Zone A key: bits:ratelimit:zone:http://app.ezeebits.com    -> count, TTL 1m
Zone B key: bits:ratelimit:zone:http://app.r2.ezeebits.com  -> count, TTL 2m
```

Zone A wait pannaalum / reset aanaalum, **Zone B affect aagaadhu**. Rendu window-um
avanga sontha first-hit la start aagum.

---

## 7. Parallel-a eppadi? (goroutines)

`WORKER_CONCURRENCY` (default 10) goroutines onnaave queue la irundhu pull pannum.
Oru goroutine Zone A ku wait pannittu irundhaalum, **matha goroutines** vera
zones-a parallel-a process pannum.

```
Goroutine-1: Zone A hit 11 -> blocked -> window reset varaikum wait
Goroutine-2: Zone B task   -> allowed -> BITS call   (parallel)
Goroutine-3: Zone C task   -> allowed -> BITS call   (parallel)
```

**Tradeoff:** ellame Zone A message-a irundhu, ella 10 goroutine-um Zone A ku wait
pannina, athukku pinnaadi queue la irukkura vera zone tasks konjam delay aagum.
Idhu wait-in-place design oda inherent nature (requeue illatha karanam).

---

## 8. Distributed-a eppadi safe? (Dragonfly)

Ovvoru hit-ku oru atomic Lua script Dragonfly la run aagudhu:

```lua
count = INCR  bits:ratelimit:zone:<zoneURL>
if count == 1 then
    PEXPIRE  <key>  <window-ms>    -- first hit window start pannudhu
end
return {count, PTTL(<key>)}
```

- **INCR atomic** — 50 worker onnaave same zone hit panna, 1,2,3...50 nu clean-a
  varum. Rendu worker sethu quota-a break panna mudiyaadhu.
- **INCR + PEXPIRE ondrey script** — count um expiry um sethu set aagudhu, so
  crash aanaalum TTL illatha counter maadhiri prachanai varaadhu.

Counter Dragonfly la iruku, adhanaala quota **ella goroutine + ella worker
instance** ku common-a work aagum.

`DRAGONFLY_ADDRESS` blank-a irundha -> **in-memory** limiter (oru instance ku
mattum), same logic mutex + map la.

---

## 9. Config (`orbitplusworker/.env`)

```
# Default: ella zone ku 1 nimisham la 10 hits
WORKER_BITS_RATE_LIMIT=10/1m

# Per-zone override: zoneURL=hits/window (comma separated)
# WORKER_BITS_RATE_LIMIT_OVERRIDES=http://app.ezeebits.com=10/1m,http://app.r2.ezeebits.com=10/2m

# Distributed state (multi-instance). Blank-a vitta in-memory use aagum.
DRAGONFLY_ADDRESS=localhost:6379
DRAGONFLY_PASSWORD=
DRAGONFLY_DATABASE=0
DRAGONFLY_CONNECTION_TIMEOUT=5s
```

`WORKER_BITS_RATE_LIMIT` set panname illa na -> rate limiting **off**.

> Note: pazhaya `WORKER_BITS_RATE_LIMIT_REQUEUE_BACKOFF` setting eduthuttom —
> ippo requeue illa, adhanaala andha config venaam.

---

## 10. Eppadi check panradhu?

### (a) Unit test (infra venaam)

```powershell
go test ./internal/application/worker/ -run RateLimit -v
```

### (b) Live-a Dragonfly la counter paakradhu

```
redis-cli -p 6379
> KEYS bits:ratelimit:zone:*
> GET  bits:ratelimit:zone:http://app.ezeebits.com   # ippo count
> PTTL bits:ratelimit:zone:http://app.ezeebits.com   # window la baaki milliseconds
```

Count 1..10 varaikum eeru, aprom key expire aagi reset aagum.

### (c) End-to-end (worker logs)

Dragonfly + RabbitMQ up pannunga, quota-vida jaasti message oru zone ku podunga.
Worker log la varum:

```
TripDetails refresh waiting for zone rate limit  zoneURL=http://app.ezeebits.com retryAfter=... wait=...
```

Window reset aana piragu athe task process aagum. Adhe time la vera zone tasks
parallel-a process aagum.

---

## 11. Code files

| File | Enna irukku |
|------|-------------|
| `internal/application/worker/ratelimit.go` | `RateLimit{Hits, Window}`, policy, in-memory window counter |
| `internal/infrastructure/dragonfly/rate_limiter.go` | Distributed atomic INCR+PEXPIRE Lua counter |
| `internal/application/worker/worker.go` | `Handle` la gate + `awaitZoneRateLimit` (wait-in-place) |
| `internal/application/worker/config.go` | `hits/window` parse (`parseRateLimit`) |
| `.env` | Settings |

---

## Summary (oru vari la)

Gate BITS-ku munnaadi -> ovvoru zone ku atomic counter + TTL window -> quota
ukkulla allowed -> quota thaandina **athe goroutine wait pannum** (requeue illa),
window reset aana piragu athe task process pannum -> ovvoru zone thani-a, onnu
innondrai block panname illa.
