# COMMAND_LIST (PowerShell) — Task Manager

Заменяет старый список под `tasks` (json-заглушку) на реальные модули. Порядок важен: сначала **логин**, потом запросы с `Authorization: Bearer`.

## Подготовка
```env
$base = "http://localhost:8080"
$env:DATABASE_URL = "postgres://app:secret@localhost:5432/taskmanager?sslmode=disable"
$env:JWT_SECRET   = "dev-secret-min-32-bytes-long-padding!!"
$env:HTTP_PORT    = "8080"
```
# Утилита: удобный вывод (статус + тело)
function Show($resp) {
    "status: $($resp.StatusCode)"
    $resp.Content
}

> Перед первым запросом — засеять `root`/`admin123` (см. README, раздел «Run»). Все команды ниже считают, что такой пользователь есть.

---

## 0) Seed первого пользователя (однократно)
```Powershell
$HASH = go run ./cmd/hashpw "admin123"

docker compose exec postgres psql -U app -d taskmanager -c @"
INSERT INTO users (login, password, name, role_id)
VALUES ('root', '$HASH', 'Root', (SELECT id FROM roles WHERE name='super_admin'))
ON CONFLICT (login) DO NOTHING;
"@
```
---

## 1) Auth — логин и токены
```Powershell
# 1.1) login success → 200 + access_token/refresh_token
$body = '{"login":"root","password":"admin123"}'
$resp = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"; "X-Request-ID"="demo-req-123"} `
    -Body $body
Show ([pscustomobject]@{ StatusCode=200; Content=($resp | ConvertTo-Json -Depth 5) })

# сохраняем токены в переменные сессии
$login   = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body $body
$access  = $login.access_token
$refresh = $login.refresh_token
$auth    = @{ "Authorization" = "Bearer $access" }

# 1.2) /me — 200, роль и scopes
Show (Invoke-WebRequest -Uri "$base/api/v1/auth/me" -Method Get -Headers $auth)

# 1.3) login неверный пароль → 401 invalid_credentials
$bad = '{"login":"root","password":"nope"}'
Show (Invoke-WebRequest -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body $bad) 2>$null

# 1.4) refresh → 200 (ротация)
$rbody = "{`"refresh_token`":`"$refresh`"}"
$pair  = Invoke-RestMethod -Uri "$base/api/v1/auth/refresh" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body $rbody
$access  = $pair.access_token
$refresh = $pair.refresh_token
$auth    = @{ "Authorization" = "Bearer $access" }

# 1.5) reuse СТАРОГО refresh → 401 token_reuse (и все сессии отозваны)
Show (Invoke-WebRequest -Uri "$base/api/v1/auth/refresh" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body "{`"refresh_token`":`"<старый_значение>`"}") 2>$null
```
> После **reuse** все refresh-токены пользователя отозваны — понадобится снова `/login`, чтобы продолжить.

---

## 2) Reviews — отзывы
```Powershell
# 2.1) создание (публично) → 201
$body = '{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"Не работает проектор","description":"в ауд. 101","tags":["bug"]}'
$resp = Invoke-WebRequest -Uri "$base/api/v1/reviews" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body $body
$reviewId = ($resp.Content | ConvertFrom-Json).id
Show $resp

# 2.2) невалидный JSON → 400 bad_request
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body '{"title":') 2>$null

# 2.3) неизвестное поле (strict JSON) → 400 bad_request
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"key_id":1,"title":"X","extra":1}') 2>$null

# 2.4) валидация: пустой title → 400 validation_error
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"key_id":1,"type_forum_id":1,"title":""}') 2>$null

# 2.5) список (публично) → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews?limit=5" -Method Get)

# 2.6) деталь — под auth + feedback:write (у super_admin *:* покрывает)
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews/$reviewId" -Method Get -Headers $auth)

# 2.7) смена статуса new→in_progress → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews/$reviewId/status" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"status_id":2}')

# 2.8) недопустимый переход in_progress→new → 409 invalid_transition
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews/$reviewId/status" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"status_id":1}') 2>$null

# 2.9) PATCH → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews/$reviewId" -Method Patch `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"title":"Уточнено","tags":["bug","urgent"]}')

# 2.10) DELETE → 204
Show (Invoke-WebRequest -Uri "$base/api/v1/reviews/$reviewId" -Method Delete -Headers $auth)
```
---

## 3) Events — мероприятия
```Powershell
# 3.1) создание → 201 (super_admin через *:*)
$body = '{"title":"Go Meetup","description":"talks","date_started":"2026-05-01T18:00:00Z","duration_seconds":7200,"place":"Amsterdam"}'
$resp = Invoke-WebRequest -Uri "$base/api/v1/events" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body $body
$eventId = ($resp.Content | ConvertFrom-Json).id
Show $resp

# 3.2) без auth → 401
Show (Invoke-WebRequest -Uri "$base/api/v1/events" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body $body) 2>$null

# 3.3) публичный список → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/events?limit=10" -Method Get)

# 3.4) фильтр по датам
Show (Invoke-WebRequest -Uri "$base/api/v1/events?from=2026-01-01&to=2026-12-31" -Method Get)
Show (Invoke-WebRequest -Uri "$base/api/v1/events?upcoming=true" -Method Get)

# 3.5) невалидный диапазон from>to → 400
Show (Invoke-WebRequest -Uri "$base/api/v1/events?from=2026-12-01&to=2026-01-01" -Method Get) 2>$null

# 3.6) PATCH → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/events/$eventId" -Method Patch `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"place":"Rotterdam"}')

# 3.7) DELETE → 204
Show (Invoke-WebRequest -Uri "$base/api/v1/events/$eventId" -Method Delete -Headers $auth)
```
---

## 4) Rooms — карта аудиторий
```Powershell
# 4.1) создание → 201
$body = '{"number":"A101","floor":1,"building":"Main","longitude":4.9,"latitude":52.3,"description":"lab"}'
$resp = Invoke-WebRequest -Uri "$base/api/v1/rooms" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body $body
$roomId = ($resp.Content | ConvertFrom-Json).id
Show $resp

# 4.2) невалидные координаты → 400
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"number":"B1","floor":1,"building":"N","longitude":200,"latitude":52}') 2>$null

# 4.3) список (публично) → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms?building=Main" -Method Get)

# 4.4) availability default free → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms/$roomId/availability?date=2026-06-01" -Method Get)

# 4.5) PATCH availability → busy → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms/$roomId/availability" -Method Patch `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"date":"2026-06-01","status":"busy"}')

# 4.6) GET → busy
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms/$roomId/availability?date=2026-06-01" -Method Get)

# 4.7) невалидный статус → 400
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms/$roomId/availability" -Method Patch `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"date":"2026-06-01","status":"maybe"}') 2>$null

# 4.8) неизвестный room → 404
Show (Invoke-WebRequest -Uri "$base/api/v1/rooms/99999/availability?date=2026-06-01" -Method Get) 2>$null
```
---

## 5) Teachers — преподаватели
```Powershell
# 5.1) создание с room_id → 201 (с вложенным room в ответе)
$body = '{"full_name":"Иванов Иван Иванович","department":"CS","position":"Доцент","contacts":{"email":"ivanov@univ.ru","telegram":"@ivanov"},"room_id":' + $roomId + '}'
$resp = Invoke-WebRequest -Uri "$base/api/v1/teachers" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body $body
$teacherId = ($resp.Content | ConvertFrom-Json).id
Show $resp

# 5.2) невалидный email в contacts → 400
Show (Invoke-WebRequest -Uri "$base/api/v1/teachers" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"full_name":"X","department":"Y","position":"Z","contacts":{"email":"not-email"}}') 2>$null

# 5.3) список (публично) → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/teachers?department=CS&limit=10" -Method Get)

# 5.4) поиск по ФИО
Show (Invoke-WebRequest -Uri "$base/api/v1/teachers?q=иван" -Method Get)

# 5.5) PATCH отвязать от кабинета (room_id=0) → 200, room=null
Show (Invoke-WebRequest -Uri "$base/api/v1/teachers/$teacherId" -Method Patch `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"room_id":0}')

# 5.6) DELETE → 204
Show (Invoke-WebRequest -Uri "$base/api/v1/teachers/$teacherId" -Method Delete -Headers $auth)
```
---

## 6) QA — вопрос-ответ (workflow)

Нужны **три** пользователя: `root` (super_admin), `mod` (moderator), `admin2` (admin). Создаём через `/users` (см. раздел 8) или напрямую SQL.
```Powershell
# 6.1) публичное создание вопроса → 201, status_id=1 (new)
$body = '{"key_id":1,"title":"Как пройти в аудиторию 101?","description":"Не могу найти","tags":["question"]}'
$resp = Invoke-WebRequest -Uri "$base/api/v1/qa" -Method Post `
    -Headers @{"Content-Type"="application/json"} -Body $body
$qaId = ($resp.Content | ConvertFrom-Json).id
Show $resp

# 6.2) публичный список — пусто (вопрос ещё new, не published)
Show (Invoke-WebRequest -Uri "$base/api/v1/qa" -Method Get)

# 6.3) публичный GET неопубликованного → 404
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/$qaId" -Method Get) 2>$null

# ---- далее нужен модератор ----
$modLogin = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"login":"mod","password":"mod12345"}'
$modAuth = @{ "Authorization" = "Bearer $($modLogin.access_token)" }

# 6.4) очередь модератора
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/moderation/mine" -Method Get -Headers $modAuth)

# 6.5) модератор отвечает → 200, status_id=6 (pending_review)
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/$qaId/answer" -Method Post `
    -Headers ($modAuth + @{"Content-Type"="application/json"}) `
    -Body '{"answer":"Идите прямо, потом налево."}')

# ---- далее нужен админ ----
$admLogin = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"login":"admin2","password":"admin123"}'
$admAuth = @{ "Authorization" = "Bearer $($admLogin.access_token)" }

# 6.6) очередь pending_review
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/moderation/pending" -Method Get -Headers $admAuth)

# 6.7) approve → 200, status_id=7 (published), модератору уходит qa_published
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/$qaId/approve" -Method Post -Headers $admAuth)

# 6.8) публичный GET теперь → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/$qaId" -Method Get)

# 6.9) invalid transition: approve published → 409
Show (Invoke-WebRequest -Uri "$base/api/v1/qa/$qaId/approve" -Method Post -Headers $admAuth) 2>$null
```
---

## 7) Notifications — уведомления
```Powershell
# 7.1) уведомления модератора (должно быть qa_published после approve)
Show (Invoke-WebRequest -Uri "$base/api/v1/notifications" -Method Get -Headers $modAuth)

# 7.2) непрочитанные
Show (Invoke-WebRequest -Uri "$base/api/v1/notifications?unread=true" -Method Get -Headers $modAuth)

# 7.3) отметить прочитанным → 200
$notifs = Invoke-RestMethod -Uri "$base/api/v1/notifications" -Method Get -Headers $modAuth
$notifId = $notifs.items[0].id
Show (Invoke-WebRequest -Uri "$base/api/v1/notifications/$notifId/read" -Method Patch -Headers $modAuth)

# 7.4) чужое уведомление (root пытается пометить notif модератора) → 404
Show (Invoke-WebRequest -Uri "$base/api/v1/notifications/$notifId/read" -Method Patch -Headers $auth) 2>$null

# 7.5) без auth → 401
Show (Invoke-WebRequest -Uri "$base/api/v1/notifications" -Method Get) 2>$null
```
---

## 8) Users — управление (только super_admin)

```Powershell
# 8.1) создать модератора → 201 (login/moderator уже засеян ранее)
$roleIdMod = (Invoke-RestMethod -Uri "$base/api/v1/users/roles" -Headers $auth).items `
    | Where-Object { $_.name -eq "moderator" } | Select-Object -First 1 -ExpandProperty id

$body = '{"login":"mod","password":"mod12345","name":"Moderator","role_id":' + $roleIdMod + '}'
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body $body) 2>$null
# 201, либо 409 login_exists — если уже есть

# 8.2) создать админа → 201
$roleIdAdm = (Invoke-RestMethod -Uri "$base/api/v1/users/roles" -Headers $auth).items `
    | Where-Object { $_.name -eq "admin" } | Select-Object -First 1 -ExpandProperty id
$body = '{"login":"admin2","password":"admin123","name":"Admin","role_id":' + $roleIdAdm + '}'
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body $body) 2>$null

# 8.3) список пользователей (super_admin) → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/users?limit=20" -Method Get -Headers $auth)

# 8.4) справочник ролей → 200
Show (Invoke-WebRequest -Uri "$base/api/v1/users/roles" -Method Get -Headers $auth)

# 8.5) короткий пароль → 400
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"login":"x","password":"123","name":"X","role_id":5}') 2>$null

# 8.6) дубликат login → 409 login_exists
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"login":"mod","password":"mod12345","name":"Dup","role_id":5}') 2>$null

# 8.7) несуществующая роль → 400 role_not_found
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"login":"y","password":"password1","name":"Y","role_id":9999}') 2>$null

# 8.8) удалить себя → 403 cannot_delete_self
$me = Invoke-RestMethod -Uri "$base/api/v1/auth/me" -Method Get -Headers $auth
Show (Invoke-WebRequest -Uri "$base/api/v1/users/$($me.id)" -Method Delete -Headers $auth) 2>$null

# 8.9) модератор пытается создать user → 403 forbidden (role)
Show (Invoke-WebRequest -Uri "$base/api/v1/users" -Method Post `
    -Headers ($modAuth + @{"Content-Type"="application/json"}) `
    -Body '{"login":"z","password":"password1","name":"Z","role_id":5}') 2>$null
```
---

## 9) Общие ошибки и контракт
```powershell
# 9.1) неизвестный путь → 404 not_found (JSON)
Show (Invoke-WebRequest -Uri "$base/api/v1/unknown" -Method Get) 2>$null

# 9.2) Method Not Allowed → 405
Show (Invoke-WebRequest -Uri "$base/api/v1/events/1" -Method Put `
    -Headers ($auth + @{"Content-Type"="application/json"}) -Body '{}') 2>$null

# 9.3) health
Show (Invoke-WebRequest -Uri "$base/healthz" -Method Get)

# 9.4) клиентский X-Request-ID пробрасывается в ответ
$r = Invoke-WebRequest -Uri "$base/api/v1/events" -Method Get `
    -Headers @{"X-Request-ID"="demo-req-123"}
"response X-Request-ID: $($r.Headers['X-Request-ID'])"
# → demo-req-123

# 9.5) генерация X-Request-ID, если клиент не передал
$r = Invoke-WebRequest -Uri "$base/api/v1/events" -Method Get
"generated X-Request-ID: $($r.Headers['X-Request-ID'])"
# → непустой uuid
```
---

## 10) Таймаут

 10.1) на reviews RequestTimeout = 2s; медленный запрос → 408 timeout
 В новых модулях ?delay= не поддерживается — это учебная фича старого tasks.
 Если хочется проверить RequestTimeoutMiddleware — временный time.Sleep(3*time.Second)
 в events.list или rooms.list, затем:
```powershell
Show (Invoke-WebRequest -Uri "$base/api/v1/events" -Method Get) 2>$null
#   # → 408 timeout
```

---

## Сводка проверяемых кодов

| Код | Где проверить |
|---|---|
| **200** | любой успешный `GET`, `PATCH`, `POST .../status`, `.../approve` |
| **201** | `POST /reviews`, `/events`, `/rooms`, `/teachers`, `/users`, `/qa` (публично) |
| **204** | `DELETE`, `POST /auth/logout` |
| **400** | bad JSON, unknown field, validation, невалидный диапазон/дата |
| **401** | без Bearer, неверный пароль, просроченный/reuse refresh |
| **403** | scope/role не хватает (`moderator → /users`) |
| **404** | несуществующий ID, неопубликованный Q&A, чужое уведомление |
| **405** | `PUT /events/{id}` (только `PATCH`) |
| **408** | таймаут (требует искусственной задержки в хендлере) |
| **409** | invalid status transition, duplicate login, delete self |
| **413** | тело > 1 MB (`BodyLimit`) |

---

## Скрипт одним файлом

Сохрани как `testdata/smoke.ps1`:

```powershell
# testdata/smoke.ps1
$base = "http://localhost:8080"
$ErrorActionPreference = "Continue"

function Step($name) { "`n=== $name ===" }

Step "health"
(Invoke-WebRequest -Uri "$base/healthz").Content

Step "login"
$login = Invoke-RestMethod -Uri "$base/api/v1/auth/login" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"login":"root","password":"admin123"}'
$auth = @{ "Authorization" = "Bearer $($login.access_token)" }
"/me: " + (Invoke-RestMethod -Uri "$base/api/v1/auth/me" -Headers $auth).role

Step "reviews"
$rv = Invoke-RestMethod -Uri "$base/api/v1/reviews" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"Smoke"}'
"created review id=$($rv.id) status=$($rv.status_id)"

Step "events"
$ev = Invoke-RestMethod -Uri "$base/api/v1/events" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"title":"Smoke","date_started":"2026-05-01T10:00:00Z","duration_seconds":3600}'
"created event id=$($ev.id)"

Step "rooms"
$rm = Invoke-RestMethod -Uri "$base/api/v1/rooms" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"number":"Z1","floor":1,"building":"Test","longitude":0,"latitude":0}'
"created room id=$($rm.id)"

Step "teachers"
$tch = Invoke-RestMethod -Uri "$base/api/v1/teachers" -Method Post `
    -Headers ($auth + @{"Content-Type"="application/json"}) `
    -Body '{"full_name":"Smoke Tester","department":"QA","position":"Tester"}'
"created teacher id=$($tch.id)"

Step "qa (public create)"
$q = Invoke-RestMethod -Uri "$base/api/v1/qa" -Method Post `
    -Headers @{"Content-Type"="application/json"} `
    -Body '{"key_id":1,"title":"Smoke Q"}'
"created qa id=$($q.id) status=$($q.status_id)"

Step "public list checks"
"reviews:    " + (Invoke-RestMethod -Uri "$base/api/v1/reviews").total
"events:     " + (Invoke-RestMethod -Uri "$base/api/v1/events").total
"rooms:      " + (Invoke-RestMethod -Uri "$base/api/v1/rooms").total
"teachers:   " + (Invoke-RestMethod -Uri "$base/api/v1/teachers").total
"qa (published only): " + (Invoke-RestMethod -Uri "$base/api/v1/qa").total

Step "negative: no auth on mutations"
try { Invoke-WebRequest -Uri "$base/api/v1/events" -Method Post `
        -Headers @{"Content-Type"="application/json"} `
        -Body '{"title":"X","date_started":"2026-05-01T10:00:00Z"}' | Out-Null
} catch { "status: $([int]$_.Exception.Response.StatusCode)" }
# → 401
```
Запуск:

```powershell 
-ExecutionPolicy Bypass -File testdata\smoke.ps1
```

Если что-то падает на конкретной команде — пришли её вывод целиком, добьём.