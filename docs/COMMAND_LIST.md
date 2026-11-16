Справочник API и команд тестирования (Task Manager)

> [!IMPORTANT] 
> / Предупреждение: Данный справочник, примеры команд (curl / curl.exe) и JSON-ответы были сгенерированы искусственным интеллектом (ИИ) на основе анализа исходного кода и могут содержать неточности или ошибки (например, в экранировании символов, локальных ID сущностей или параметрах окружения). Перед использованием в автоматизированных скриптах проверяйте команды вручную.

Данный документ содержит полный перечень маршрутов API, примеры выполнения запросов для Linux / macOS (Bash) и Windows (PowerShell), а также ожидаемые HTTP-коды и форматы ответов при успешном выполнении и при ошибках.
> [!IMPORTANT]
>  В PowerShell команда `curl` по умолчанию является псевдонимом (alias) для `Invoke-WebRequest`, который имеет другой синтаксис. Чтобы использовать классический `curl`, в PowerShell необходимо вызывать **`curl.exe`** и экранировать кавычки внутри JSON. Все команды ниже адаптированы специально под это.
---
Оглавление и навигация
1. Аутентификация и профиль (`/api/v1/auth`)
Получение токенов (Login)
Сохранение токенов в переменные окружения
Получить информацию о себе (Me)
Обновление токенов (Refresh)
Выход из системы (Logout)
2. Уведомления (`/api/v1/notifications`)
Получить список всех своих уведомлений
Получить непрочитанные уведомления с пагинацией
Пометить уведомление прочитанным
3. Отзывы и предложения (`/api/v1/reviews`)
Создать отзыв
Получить список отзывов с фильтрацией
Получить конкретный отзыв по ID
Изменить статус отзыва
Частично обновить отзыв
Удалить отзыв
4. Мероприятия (`/api/v1/events`)
Создать мероприятие
Получить список мероприятий
Получить карточку мероприятия по ID
Частично обновить мероприятие
Удалить мероприятие
5. Карта аудиторий (`/api/v1/rooms`)
Создать аудиторию
Получить список аудиторий с фильтрами
Получить информацию об аудитории
Получить занятость аудитории
Изменить статус занятости аудитории
Обновить данные аудитории
Удалить аудиторию
6. Преподаватели (`/api/v1/teachers`)
Создать преподавателя
Получить список преподавателей с фильтрами и поиском
Получить карточку преподавателя по ID
Обновить данные преподавателя или отвязать от кабинета
Удалить преподавателя
7. Вопрос-ответ / Q&A (`/api/v1/qa`)
Задать вопрос (гость или пользователь)
Посмотреть очереди модерации
Ответить на вопрос
Посмотреть очередь на проверку
Одобрить или отклонить ответ
Публичный просмотр опубликованных вопросов
8. Управление пользователями (`/api/v1/users`)
Получить справочник доступных ролей
Создать нового пользователя
Получить список пользователей с поиском и фильтром
Получить карточку пользователя по ID
Обновить данные пользователя
Удалить пользователя
9. Общие ошибки маршрутизации и авторизации
---
1. Аутентификация и профиль (`/api/v1/auth`)
Получение токенов (Login)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "login": "admin",
    "password": "admin123"
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/auth/login `
  -H "Content-Type: application/json" `
  -d '{\"login\": \"admin\", \"password\": \"admin123\"}'
```
Успешный ответ (`200 OK`):
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "dGhpcy1pcy1hLXJhbmRvbS1vcGFxdWUtdG9rZW4",
  "token_type": "Bearer",
  "expires_in": 900
}
```
Ошибки:
`400 Bad Request` — невалидный JSON или нарушены правила валидации (`login` < 3 символов, пустой пароль):
```json
{
  "api_error": {
    "code": "validation_error",
    "message": "Validation failed",
    "request_id": "a1b2c3d4e5f6",
    "details": [{"field": "Login", "rule": "min"}]
  }
}
```
`401 Unauthorized` — неверный логин или пароль:
```json
{
  "api_error": {
    "code": "invalid_credentials",
    "message": "Invalid login or password",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
---
Сохранение токенов в переменные окружения
`linux / macos: bash`
```bash
export ACCESS_TOKEN="вставь_сюда_access_token"
export REFRESH_TOKEN="вставь_сюда_refresh_token"
```
`windows: powershell`
```powershell
$env:ACCESS_TOKEN="вставь_сюда_access_token"
$env:REFRESH_TOKEN="вставь_сюда_refresh_token"
```
---
Получить информацию о себе (Me)
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/auth/me `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
```json
{
  "id": 1,
  "login": "admin",
  "name": "Главный Администратор",
  "role": "super_admin",
  "scopes": ["*:*"]
}
```
Ошибки:
`401 Unauthorized` — отсутствует заголовок `Authorization` или токен истёк (`code: "unauthorized"` / `"invalid_token"`).
---
Обновление токенов (Refresh)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d "{
    \"refresh_token\": \"$REFRESH_TOKEN\"
  }"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/auth/refresh `
  -H "Content-Type: application/json" `
  -d "{`"refresh_token`": `"$env:REFRESH_TOKEN`"}"
```
Успешный ответ (`200 OK`):
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "bmV3LXJvdGF0ZWQtcmVmcmVzaC10b2tlbg",
  "token_type": "Bearer",
  "expires_in": 900
}
```
Ошибки:
`401 Unauthorized` — токен не найден или истёк (`code: "invalid_token"`), либо обнаружено повторное использование уже отозванного токена (блокируются все сессии пользователя):
```json
{
  "api_error": {
    "code": "token_reuse",
    "message": "Refresh token reuse detected, all sessions revoked",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
---
Выход из системы (Logout)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d "{
    \"refresh_token\": \"$REFRESH_TOKEN\"
  }"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/auth/logout `
  -H "Content-Type: application/json" `
  -d "{`"refresh_token`": `"$env:REFRESH_TOKEN`"}"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа (операция идемпотентна).
Ошибки:
`400 Bad Request` — не передан `refresh_token` (`code: "validation_error"`).
---
2. Уведомления (`/api/v1/notifications`)
Получить список всех своих уведомлений
`linux / macos: bash -> curl`
```bash
curl -i -X GET "http://localhost:8080/api/v1/notifications" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET "http://localhost:8080/api/v1/notifications" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
```json
{
  "items": [
    {
      "id": 1,
      "user_id": 1,
      "kind": "qa_published",
      "payload": {
        "qa_id": 1,
        "status": "published",
        "title": "Как получить справку об обучении?"
      },
      "created_at": "2026-11-16T14:20:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```
---
Получить непрочитанные уведомления с пагинацией
`linux / macos: bash -> curl`
```bash
curl -i -X GET "http://localhost:8080/api/v1/notifications?unread=true&limit=10&offset=0" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET "http://localhost:8080/api/v1/notifications?unread=true&limit=10&offset=0" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` с непрочитанными уведомлениями (`read_at` отсутствует).
---
Пометить уведомление прочитанным
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH "http://localhost:8080/api/v1/notifications/1/read" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH "http://localhost:8080/api/v1/notifications/1/read" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
```json
{
  "id": 1,
  "user_id": 1,
  "kind": "qa_published",
  "payload": {"qa_id": 1, "status": "published", "title": "Как получить справку об обучении?"},
  "read_at": "2026-11-16T14:25:00Z",
  "created_at": "2026-11-16T14:20:00Z"
}
```
Ошибки:
`400 Bad Request` — некорректный ID в URL (`code: "bad_request"`).
`404 Not Found` — уведомление не найдено или принадлежит другому пользователю (защита от IDOR):
```json
{
  "api_error": {
    "code": "not_found",
    "message": "Notification not found",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
---
3. Отзывы и предложения (`/api/v1/reviews`)
Создать отзыв
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "key_id": 1,
    "sender_id": 1,
    "type_forum_id": 1,
    "title": "Не работает проектор в 305 аудитории",
    "description": "При подключении по HDMI нет сигнала",
    "tags": ["оборудование", "аудитория"]
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/reviews `
  -H "Content-Type: application/json" `
  -d '{\"key_id\": 1, \"sender_id\": 1, \"type_forum_id\": 1, \"title\": \"Не работает проектор в 305 аудитории\", \"description\": \"При подключении по HDMI нет сигнала\", \"tags\": [\"оборудование\", \"аудитория\"]}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/reviews/1`.
```json
{
  "id": 1,
  "key_id": 1,
  "sender_id": 1,
  "type_forum_id": 1,
  "title": "Не работает проектор в 305 аудитории",
  "description": "При подключении по HDMI нет сигнала",
  "tags": ["оборудование", "аудитория"],
  "status_id": 1,
  "created_at": "2026-11-16T14:30:00Z"
}
```
Ошибки:
`400 Bad Request` — ошибка валидации полей или передано неизвестное поле JSON (`code: "validation_error"` / `"bad_request"`).
---
Получить список отзывов с фильтрацией
`linux / macos: bash -> curl`
```bash
# Все отзывы с пагинацией
curl -i -X GET "http://localhost:8080/api/v1/reviews?limit=10&offset=0"

# Фильтр по статусу (1 = new) и тегу
curl -i -X GET "http://localhost:8080/api/v1/reviews?status_id=1&tag=оборудование"
```
`windows: powershell -> curl.exe`
```powershell
# Все отзывы с пагинацией
curl.exe -i -X GET "http://localhost:8080/api/v1/reviews?limit=10&offset=0"

# Фильтр по статусу (1 = new) и тегу
curl.exe -i -X GET "http://localhost:8080/api/v1/reviews?status_id=1&tag=оборудование"
```
Успешный ответ (`200 OK`):
```json
{
  "items": [
    {
      "id": 1,
      "key_id": 1,
      "sender_id": 1,
      "type_forum_id": 1,
      "title": "Не работает проектор в 305 аудитории",
      "description": "При подключении по HDMI нет сигнала",
      "tags": ["оборудование", "аудитория"],
      "status_id": 1,
      "created_at": "2026-11-16T14:30:00Z"
    }
  ],
  "total": 1,
  "limit": 10,
  "offset": 0
}
```
---
Получить конкретный отзыв по ID
(Требуется скоуп `feedback:write` или `feedback:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/reviews/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/reviews/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
Возвращает JSON-объект `Review`.
Ошибки:
`403 Forbidden` — у пользователя нет скоупа `feedback:write` (`code: "forbidden"`).
`404 Not Found` — отзыв с указанным ID не найден (`code: "not_found"`).
---
Изменить статус отзыва
(Допустимые переходы: `1 (new) -> 2 (in_progress) | 4 (rejected)`, `2 (in_progress) -> 3 (resolved) | 4 (rejected)`)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/reviews/1/status \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "status_id": 2
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/reviews/1/status `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"status_id\": 2}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный объект `Review` с новым `status_id`.
Ошибки:
`409 Conflict` — недопустимый переход статусов (например, из `resolved` в `in_progress` или повторная установка текущего статуса):
```json
{
  "api_error": {
    "code": "invalid_transition",
    "message": "Invalid status transition",
    "request_id": "a1b2c3d4e5f6",
    "details": {"error": "reviews: invalid status transition: 3 -> 2"}
  }
}
```
`404 Not Found` — отзыв не найден (`code: "not_found"`).
---
Частично обновить отзыв
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/reviews/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Обновленный заголовок отзыва",
    "tags": ["срочно", "оборудование"]
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/reviews/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"title\": \"Обновленный заголовок отзыва\", \"tags\": [\"срочно\", \"оборудование\"]}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный JSON-объект `Review`.
Ошибки:
`404 Not Found` — отзыв не найден (`code: "not_found"`).
---
Удалить отзыв
`linux / macos: bash -> curl`
```bash
curl -i -X DELETE http://localhost:8080/api/v1/reviews/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X DELETE http://localhost:8080/api/v1/reviews/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа.
Ошибки:
`404 Not Found` — отзыв не найден (`code: "not_found"`).
---
4. Мероприятия (`/api/v1/events`)
Создать мероприятие
(Требуется скоуп `events:write` или `events:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/events \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "День открытых дверей",
    "description": "Встреча с преподавателями и экскурсия по лабораториям",
    "date_started": "2026-12-01T15:00:00Z",
    "duration_seconds": 7200,
    "place": "Актовый зал, корпус А"
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/events `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"title\": \"День открытых дверей\", \"description\": \"Встреча с преподавателями и экскурсия по лабораториям\", \"date_started\": \"2026-12-01T15:00:00Z\", \"duration_seconds\": 7200, \"place\": \"Актовый зал, корпус А\"}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/events/1`.
```json
{
  "id": 1,
  "title": "День открытых дверей",
  "description": "Встреча с преподавателями и экскурсия по лабораториям",
  "date_started": "2026-12-01T15:00:00Z",
  "duration_seconds": 7200,
  "place": "Актовый зал, корпус А",
  "created_at": "2026-11-16T14:35:00Z"
}
```
Ошибки:
`400 Bad Request` — неверный формат `date_started` (требуется RFC3339) или отрицательное `duration_seconds` (`code: "validation_error"` / `"bad_request"`).
---
Получить список мероприятий
`linux / macos: bash -> curl`
```bash
# Все мероприятия с пагинацией
curl -i -X GET "http://localhost:8080/api/v1/events?limit=10&offset=0"

# Только предстоящие мероприятия
curl -i -X GET "http://localhost:8080/api/v1/events?upcoming=true"

# Фильтр по диапазону дат (RFC3339 или YYYY-MM-DD)
curl -i -X GET "http://localhost:8080/api/v1/events?from=2026-11-01&to=2026-12-31"
```
`windows: powershell -> curl.exe`
```powershell
# Все мероприятия с пагинацией
curl.exe -i -X GET "http://localhost:8080/api/v1/events?limit=10&offset=0"

# Только предстоящие мероприятия
curl.exe -i -X GET "http://localhost:8080/api/v1/events?upcoming=true"

# Фильтр по диапазону дат (RFC3339 или YYYY-MM-DD)
curl.exe -i -X GET "http://localhost:8080/api/v1/events?from=2026-11-01&to=2026-12-31"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` с массивом `items`, `total`, `limit`, `offset`.
Ошибки:
`400 Bad Request` — неверный формат даты в `from`/`to` или дата `to` раньше даты `from`:
```json
{
  "api_error": {
    "code": "bad_request",
    "message": "'to' must be greater than or equal to 'from'",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
---
Получить карточку мероприятия по ID
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/events/1
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/events/1
```
Успешный ответ (`200 OK`):
Возвращает JSON-объект `Event`.
Ошибки:
`404 Not Found` — мероприятие не найдено (`code: "not_found"`).
---
Частично обновить мероприятие
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/events/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "place": "Аудитория 404",
    "duration_seconds": 5400
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/events/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"place\": \"Аудитория 404\", \"duration_seconds\": 5400}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный JSON-объект `Event`.
Ошибки:
`404 Not Found` — мероприятие не найдено (`code: "not_found"`).
---
Удалить мероприятие
`linux / macos: bash -> curl`
```bash
curl -i -X DELETE http://localhost:8080/api/v1/events/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X DELETE http://localhost:8080/api/v1/events/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа.
Ошибки:
`404 Not Found` — мероприятие не найдено (`code: "not_found"`).
---
5. Карта аудиторий (`/api/v1/rooms`)
Создать аудиторию
(Требуется скоуп `map:write` или `map:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/rooms \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "number": "305",
    "floor": 3,
    "building": "Главный корпус",
    "longitude": 44.6817,
    "latitude": 43.0231,
    "description": "Лекционная аудитория с проектором"
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/rooms `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"number\": \"305\", \"floor\": 3, \"building\": \"Главный корпус\", \"longitude\": 44.6817, \"latitude\": 43.0231, \"description\": \"Лекционная аудитория с проектором\"}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/rooms/1`.
```json
{
  "id": 1,
  "number": "305",
  "floor": 3,
  "building": "Главный корпус",
  "longitude": 44.6817,
  "latitude": 43.0231,
  "description": "Лекционная аудитория с проектором",
  "created_at": "2026-11-16T14:40:00Z"
}
```
Ошибки:
`400 Bad Request` — не указан `number` или `building`, либо координаты выходят за пределы `[-180..180]` / `[-90..90]` (`code: "validation_error"`).
---
Получить список аудиторий с фильтрами
`linux / macos: bash -> curl`
```bash
# Все аудитории
curl -i -X GET "http://localhost:8080/api/v1/rooms"

# Фильтр по корпусу и этажу
curl -i -X GET "http://localhost:8080/api/v1/rooms?building=Главный%20корпус&floor=3&limit=20&offset=0"
```
`windows: powershell -> curl.exe`
```powershell
# Все аудитории
curl.exe -i -X GET "http://localhost:8080/api/v1/rooms"

# Фильтр по корпусу и этажу
curl.exe -i -X GET "http://localhost:8080/api/v1/rooms?building=Главный%20корпус&floor=3&limit=20&offset=0"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` с массивом аудиторий `items`.
---
Получить информацию об аудитории
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/rooms/1
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/rooms/1
```
Успешный ответ (`200 OK`):
Возвращает JSON-объект `Room`.
Ошибки:
`404 Not Found` — аудитория не найдена (`code: "not_found"`).
---
Получить занятость аудитории
`linux / macos: bash -> curl`
```bash
# На конкретную дату (YYYY-MM-DD)
curl -i -X GET "http://localhost:8080/api/v1/rooms/1/availability?date=2026-12-01"

# На сегодняшнюю дату (без параметра date)
curl -i -X GET "http://localhost:8080/api/v1/rooms/1/availability"
```
`windows: powershell -> curl.exe`
```powershell
# На конкретную дату (YYYY-MM-DD)
curl.exe -i -X GET "http://localhost:8080/api/v1/rooms/1/availability?date=2026-12-01"

# На сегодняшнюю дату (без параметра date)
curl.exe -i -X GET "http://localhost:8080/api/v1/rooms/1/availability"
```
Успешный ответ (`200 OK`):
(Если запись на дату ещё не задавалась, по умолчанию возвращается `free`):
```json
{
  "room_id": 1,
  "date": "2026-12-01",
  "status": "free",
  "updated_at": "0001-01-01T00:00:00Z"
}
```
Ошибки:
`400 Bad Request` — неверный формат даты (`expected YYYY-MM-DD`).
`404 Not Found` — аудитория не найдена (`code: "not_found"`).
---
Изменить статус занятости аудитории
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/rooms/1/availability \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "date": "2026-12-01",
    "status": "busy"
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/rooms/1/availability `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"date\": \"2026-12-01\", \"status\": \"busy\"}'
```
Успешный ответ (`200 OK`):
```json
{
  "room_id": 1,
  "date": "2026-12-01",
  "status": "busy",
  "updated_at": "2026-11-16T14:42:00Z"
}
```
Ошибки:
`400 Bad Request` — статус не равен `free` или `busy`, либо неверный формат `date` (`code: "validation_error"` / `"bad_request"`).
`404 Not Found` — аудитория не найдена (`code: "not_found"`).
---
Обновить данные аудитории
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/rooms/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Компьютерный класс на 25 мест"
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/rooms/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"description\": \"Компьютерный класс на 25 мест\"}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный JSON-объект `Room`.
Ошибки:
`404 Not Found` — аудитория не найдена (`code: "not_found"`).
---
Удалить аудиторию
`linux / macos: bash -> curl`
```bash
curl -i -X DELETE http://localhost:8080/api/v1/rooms/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X DELETE http://localhost:8080/api/v1/rooms/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа.
Ошибки:
`404 Not Found` — аудитория не найдена (`code: "not_found"`).
---
6. Преподаватели (`/api/v1/teachers`)
Создать преподавателя
(Требуется скоуп `teachers:write` или `teachers:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/teachers \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Иванов Иван Иванович",
    "department": "Кафедра информатики и ВТ",
    "position": "Доцент",
    "contacts": {
      "email": "ivanov@university.ru",
      "telegram": "@ivanov_prof",
      "office_hours": "Вт 14:00-16:00"
    },
    "photo_url": "https://example.com/photos/ivanov.jpg",
    "room_id": 1
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/teachers `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"full_name\": \"Иванов Иван Иванович\", \"department\": \"Кафедра информатики и ВТ\", \"position\": \"Доцент\", \"contacts\": {\"email\": \"ivanov@university.ru\", \"telegram\": \"@ivanov_prof\", \"office_hours\": \"Вт 14:00-16:00\"}, \"photo_url\": \"https://example.com/photos/ivanov.jpg\", \"room_id\": 1}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/teachers/1`.
```json
{
  "id": 1,
  "full_name": "Иванов Иван Иванович",
  "department": "Кафедра информатики и ВТ",
  "position": "Доцент",
  "contacts": {
    "email": "ivanov@university.ru",
    "telegram": "@ivanov_prof",
    "office_hours": "Вт 14:00-16:00"
  },
  "photo_url": "https://example.com/photos/ivanov.jpg",
  "room": {
    "id": 1,
    "number": "305",
    "building": "Главный корпус"
  },
  "created_at": "2026-11-16T14:45:00Z"
}
```
Ошибки:
`400 Bad Request` — невалидный `email`, некорректный `photo_url` или не заполнены обязательные поля (`code: "validation_error"`).
---
Получить список преподавателей с фильтрами и поиском
`linux / macos: bash -> curl`
```bash
# Все преподаватели
curl -i -X GET "http://localhost:8080/api/v1/teachers"

# Поиск по ФИО (ILIKE), кафедре и аудитории
curl -i -X GET "http://localhost:8080/api/v1/teachers?q=иванов&room_id=1&limit=20&offset=0"
```
`windows: powershell -> curl.exe`
```powershell
# Все преподаватели
curl.exe -i -X GET "http://localhost:8080/api/v1/teachers"

# Поиск по ФИО (ILIKE), кафедре и аудитории
curl.exe -i -X GET "http://localhost:8080/api/v1/teachers?q=иванов&room_id=1&limit=20&offset=0"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` со списком преподавателей `items`.
---
Получить карточку преподавателя по ID
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/teachers/1
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/teachers/1
```
Успешный ответ (`200 OK`):
Возвращает JSON-объект `Teacher` со вложенным объектом `room` (если кабинет привязан).
Ошибки:
`404 Not Found` — преподаватель не найден (`code: "not_found"`).
---
Обновить данные преподавателя или отвязать от кабинета
(Передача `"room_id": 0` отвязывает преподавателя от кабинета)
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/teachers/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "position": "Профессор",
    "room_id": 0
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/teachers/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"position\": \"Профессор\", \"room_id\": 0}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный JSON-объект `Teacher` (поле `room` будет отсутствовать).
Ошибки:
`404 Not Found` — преподаватель не найден (`code: "not_found"`).
---
Удалить преподавателя
`linux / macos: bash -> curl`
```bash
curl -i -X DELETE http://localhost:8080/api/v1/teachers/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X DELETE http://localhost:8080/api/v1/teachers/1 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа.
Ошибки:
`404 Not Found` — преподаватель не найден (`code: "not_found"`).
---
7. Вопрос-ответ / Q&A (`/api/v1/qa`)
Задать вопрос (гость или пользователь)
`linux / macos: bash -> curl`
```bash
# От гостя (без авторизации)
curl -i -X POST http://localhost:8080/api/v1/qa \
  -H "Content-Type: application/json" \
  -d '{
    "key_id": 1,
    "title": "Как получить справку об обучении?",
    "description": "Нужна справка с гербовой печатью для военкомата",
    "tags": ["документы", "деканат"]
  }'

# От авторизованного пользователя (sender_id подставится из JWT)
curl -i -X POST http://localhost:8080/api/v1/qa \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "key_id": 1,
    "title": "Когда начинается сессия?",
    "description": "Интересует расписание зимней сессии для 2 курса"
  }'
```
`windows: powershell -> curl.exe`
```powershell
# От гостя (без авторизации)
curl.exe -i -X POST http://localhost:8080/api/v1/qa `
  -H "Content-Type: application/json" `
  -d '{\"key_id\": 1, \"title\": \"Как получить справку об обучении?\", \"description\": \"Нужна справка с гербовой печатью для военкомата\", \"tags\": [\"документы\", \"деканат\"]}'

# От авторизованного пользователя (sender_id подставится из JWT)
curl.exe -i -X POST http://localhost:8080/api/v1/qa `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"key_id\": 1, \"title\": \"Когда начинается сессия?\", \"description\": \"Интересует расписание зимней сессии для 2 курса\"}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/qa/1`.
```json
{
  "id": 1,
  "key_id": 1,
  "type_forum_id": 2,
  "title": "Как получить справку об обучении?",
  "description": "Нужна справка с гербовой печатью для военкомата",
  "tags": ["документы", "деканат"],
  "status_id": 1,
  "created_at": "2026-11-16T14:50:00Z",
  "answer": "",
  "review_comment": ""
}
```
---
Посмотреть очереди модерации
(Требуется скоуп `qa:write` или `qa:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/qa/moderation/mine \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/qa/moderation/mine `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
```json
{
  "new": [ ... ],
  "new_total": 1,
  "needs_revision": [],
  "revision_total": 0
}
```
---
Ответить на вопрос
(Переводит вопрос из `1 (new)` или `8 (needs_revision)` в `6 (pending_review)`. Требуется скоуп `qa:write`)
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/qa/1/answer \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "answer": "Справку можно заказать в личном кабинете или забрать в деканате (каб. 204) с 10:00 до 16:00."
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/qa/1/answer `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"answer\": \"Справку можно заказать в личном кабинете или забрать в деканате (каб. 204) с 10:00 до 16:00.\"}'
```
Успешный ответ (`200 OK`):
Возвращает объект `QA` со статусом `"status_id": 6`, заполненными `moderator_id`, `answer` и `answered_at`.
Ошибки:
`409 Conflict` — вопрос уже находится на проверке (`6`) или опубликован (`7`):
```json
{
  "api_error": {
    "code": "invalid_status",
    "message": "Question is not in a state that can be answered",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
`404 Not Found` — вопрос не найден (`code: "not_found"`).
---
Посмотреть очередь на проверку
(Требуется скоуп `qa:admin`)
`linux / macos: bash -> curl`
```bash
curl -i -X GET "http://localhost:8080/api/v1/qa/moderation/pending?limit=20&offset=0" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET "http://localhost:8080/api/v1/qa/moderation/pending?limit=20&offset=0" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` с вопросами в статусе `pending_review (6)`.
---
Одобрить или отклонить ответ
(Требуется скоуп `qa:admin`. После действия модератору отправляется уведомление)
`linux / macos: bash -> curl`
```bash
# Одобрить и опубликовать (status_id -> 7)
curl -i -X POST http://localhost:8080/api/v1/qa/1/approve \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# ИЛИ отклонить на доработку с комментарием (status_id -> 8)
curl -i -X POST http://localhost:8080/api/v1/qa/1/reject \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "comment": "Укажите, пожалуйста, номер телефона деканата"
  }'
```
`windows: powershell -> curl.exe`
```powershell
# Одобрить и опубликовать (status_id -> 7)
curl.exe -i -X POST http://localhost:8080/api/v1/qa/1/approve `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"

# ИЛИ отклонить на доработку с комментарием (status_id -> 8)
curl.exe -i -X POST http://localhost:8080/api/v1/qa/1/reject `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"comment\": \"Укажите, пожалуйста, номер телефона деканата\"}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный объект `QA` со статусом `7 (published)` или `8 (needs_revision)`.
Ошибки:
`409 Conflict` — вопрос не находится в статусе `pending_review (6)` (`code: "invalid_status"`).
`400 Bad Request` — при отклонении (`reject`) не передан обязательный комментарий `comment` (`code: "validation_error"`).
---
Публичный просмотр опубликованных вопросов
`linux / macos: bash -> curl`
```bash
# Список опубликованных вопросов
curl -i -X GET "http://localhost:8080/api/v1/qa?limit=10&offset=0"

# Карточка опубликованного вопроса
curl -i -X GET http://localhost:8080/api/v1/qa/1
```
`windows: powershell -> curl.exe`
```powershell
# Список опубликованных вопросов
curl.exe -i -X GET "http://localhost:8080/api/v1/qa?limit=10&offset=0"

# Карточка опубликованного вопроса
curl.exe -i -X GET http://localhost:8080/api/v1/qa/1
```
Успешный ответ (`200 OK`):
Возвращает только вопросы со статусом `published (7)`.
Ошибки:
`404 Not Found` — вопрос не существует либо ещё не опубликован (`status_id != 7`).
---
8. Управление пользователями (`/api/v1/users`)
(Весь раздел доступен только пользователям с ролью `super_admin`)
Получить справочник доступных ролей
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/users/roles \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/users/roles `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
```json
{
  "items": [
    {"id": 1, "name": "admin", "scopes": ["feedback:admin", "map:admin", "teachers:admin", "events:admin", "qa:admin"]},
    {"id": 3, "name": "client", "scopes": ["feedback:read", "map:read", "teachers:read", "events:read", "qa:read"]},
    {"id": 4, "name": "moderator", "scopes": ["feedback:read", "feedback:write", "..."]},
    {"id": 5, "name": "teacher", "scopes": ["feedback:read", "..."]},
    {"id": 6, "name": "super_admin", "scopes": ["*:*"]}
  ]
}
```
---
Создать нового пользователя
`linux / macos: bash -> curl`
```bash
curl -i -X POST http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "login": "moderator1",
    "password": "StrongPassword123",
    "name": "Смирнов Алексей",
    "role_id": 4,
    "scopes": ["events:admin"]
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X POST http://localhost:8080/api/v1/users `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"login\": \"moderator1\", \"password\": \"StrongPassword123\", \"name\": \"Смирнов Алексей\", \"role_id\": 4, \"scopes\": [\"events:admin\"]}'
```
Успешный ответ (`201 Created`):
Заголовок ответа содержит `Location: /api/v1/users/2`.
```json
{
  "id": 2,
  "login": "moderator1",
  "name": "Смирнов Алексей",
  "role_id": 4,
  "role": "moderator",
  "scopes": ["events:admin"],
  "created_at": "2026-11-16T15:00:00Z"
}
```
Ошибки:
`400 Bad Request` — пароль короче 8 символов (`validation_error`) или указан несуществующий `role_id` (`code: "role_not_found"`).
`409 Conflict` — логин уже занят:
```json
{
  "api_error": {
    "code": "login_exists",
    "message": "Login already exists",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
---
Получить список пользователей с поиском и фильтром
`linux / macos: bash -> curl`
```bash
# Все пользователи
curl -i -X GET "http://localhost:8080/api/v1/users" \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# Поиск по логину или имени (ILIKE) + фильтр по role_id
curl -i -X GET "http://localhost:8080/api/v1/users?q=смирнов&role_id=4&limit=20&offset=0" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
# Все пользователи
curl.exe -i -X GET "http://localhost:8080/api/v1/users" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"

# Поиск по логину или имени (ILIKE) + фильтр по role_id
curl.exe -i -X GET "http://localhost:8080/api/v1/users?q=смирнов&role_id=4&limit=20&offset=0" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
Возвращает объект `ListResponse` со списком пользователей `items` (хэш пароля в ответ никогда не попадает).
---
Получить карточку пользователя по ID
`linux / macos: bash -> curl`
```bash
curl -i -X GET http://localhost:8080/api/v1/users/2 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X GET http://localhost:8080/api/v1/users/2 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`200 OK`):
Возвращает JSON-объект `User`.
Ошибки:
`404 Not Found` — пользователь не найден (`code: "not_found"`).
---
Обновить данные пользователя
`linux / macos: bash -> curl`
```bash
curl -i -X PATCH http://localhost:8080/api/v1/users/2 \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Смирнов Алексей Петрович",
    "scopes": []
  }'
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X PATCH http://localhost:8080/api/v1/users/2 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{\"name\": \"Смирнов Алексей Петрович\", \"scopes\": []}'
```
Успешный ответ (`200 OK`):
Возвращает обновленный JSON-объект `User`.
Ошибки:
`400 Bad Request` — указан несуществующий `role_id` (`code: "role_not_found"`).
`404 Not Found` — пользователь не найден (`code: "not_found"`).
---
Удалить пользователя
`linux / macos: bash -> curl`
```bash
curl -i -X DELETE http://localhost:8080/api/v1/users/2 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```
`windows: powershell -> curl.exe`
```powershell
curl.exe -i -X DELETE http://localhost:8080/api/v1/users/2 `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"
```
Успешный ответ (`204 No Content`):
Пустое тело ответа.
Ошибки:
`403 Forbidden` — попытка удалить собственную учётную запись:
```json
{
  "api_error": {
    "code": "cannot_delete_self",
    "message": "Cannot delete your own account",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
`409 Conflict` — у пользователя есть связанные записи в БД (например, созданные отзывы с `ON DELETE RESTRICT`):
```json
{
  "api_error": {
    "code": "user_in_use",
    "message": "User has dependent data and cannot be deleted",
    "request_id": "a1b2c3d4e5f6"
  }
}
```
`404 Not Found` — пользователь не найден (`code: "not_found"`).
---
9. Общие ошибки маршрутизации и авторизации
Эти ошибки могут вернуться на любом маршруте API:
`401 Unauthorized` — не передан заголовок `Authorization: Bearer <token>` (`code: "unauthorized"`) или токен невалиден/просрочен (`code: "invalid_token"`).
`403 Forbidden` — недостаточно прав (`code: "forbidden"`, в `details` указывается требуемый скоуп или роль).
`404 Not Found` — обращение к несуществующему пути (`code: "not_found"`).
`405 Method Not Allowed` — HTTP-метод не поддерживается для данного пути (`code: "method_not_allowed"`).
`408 Request Timeout` — превышено время ожидания обработки запроса (`code: "timeout"`).
`413 Payload Too Large` — размер тела запроса превысил установленный лимит (`code: "payload_too_large"`).