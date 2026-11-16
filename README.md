# Task Manager (Go) — учебный REST API

Учебный проект для отработки Web/HTTP на Go: роутинг, CRUD, middleware, context, graceful shutdown, JWT-аутентификация, RBAC/scopes, работа с PostgreSQL через `database/sql` + `pgx`, миграции через `golang-migrate`, фоновые воркеры.

Домены: **отзывы**, **мероприятия**, **аудитории (карта)**, **преподаватели**, **вопрос-ответ**, **уведомления**, **управление пользователями**.

---

## Run

```bash
# 1. Зависимости
go mod tidy

# 2. PostgreSQL (docker)
docker compose up -d postgres

# 3. Переменные окружения (см. .env.example)
$env:DATABASE_URL = "postgres://app:secret@localhost:5432/taskmanager?sslmode=disable"
$env:JWT_SECRET   = "dev-secret-min-32-bytes-long-padding!!"

# 4. Запуск (миграции накатываются автоматически при старте)
go run ./cmd/task-server


# PS
Сервер стартует на :8080. Миграции 0001..0009 применяются автоматически при первом запуске.

Первый пользователь (super_admin) заводится вручную — bcrypt-хэш генерится утилитой:

```bash
$HASH = go run ./cmd/hashpw "admin123"

psql "$DATABASE_URL" -c @"
INSERT INTO users (login, password, name, role_id)
VALUES ('root', '$HASH', 'Root', (SELECT id FROM roles WHERE name='super_admin'));
"@
```