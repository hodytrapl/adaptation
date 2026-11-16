# Инструкция по интеграционному тестированию и запуску сквозного сценария (Task Manager)

> [!IMPORTANT] 
> Данный документ и тестовые скрипты были сгенерированы искусственным интеллектом (ИИ) и могут содержать опечатки, неточности или требовать адаптации под вашу версию ОС и окружения.

В этом документе приведен пошаговый сценарий сквозного тестирования API. Сценарий включает:
1. Авторизацию суперадмином и сохранение токена.
2. Создание аудитории.
3. Создание преподавателя (с привязкой к аудитории).
4. Создание отзыва.
5. Создание вопроса (Q&A) от гостя.
6. Ответ модератора на вопрос.
7. Публикацию вопроса администратором.
8. Проверку получения уведомления.

---

## Вариант 1. Linux / macOS (Bash + curl + jq)

Перед запуском убедитесь, что у вас установлены `curl` и `jq`.

```bash
#!/usr/bin/env bash
set -e

echo "=== 1. Логин суперадмином и получение ACCESS_TOKEN ==="
export ACCESS_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"login":"admin","password":"admin123"}' | jq -r '.access_token')

echo "Полученный Token: $ACCESS_TOKEN"

echo "=== 2. Создаём аудиторию (получит id=1) ==="
curl -s -i -X POST http://localhost:8080/api/v1/rooms \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"number":"305","floor":3,"building":"Главный","longitude":44.68,"latitude":43.02,"description":"Лекционная"}'
echo -e "\n"

echo "=== 3. Создаём преподавателя с привязкой к аудитории id=1 ==="
curl -s -i -X POST http://localhost:8080/api/v1/teachers \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Иванов Иван Иванович","department":"ИВТ","position":"Доцент","room_id":1}'
echo -e "\n"

echo "=== 4. Создаём отзыв (sender_id=1 — admin, type_forum_id=1 — review) ==="
curl -s -i -X POST http://localhost:8080/api/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"Отличная аудитория","tags":["map"]}'
echo -e "\n"

echo "=== 5. Задаём вопрос как гость (Q&A) ==="
curl -s -i -X POST http://localhost:8080/api/v1/qa \
  -H "Content-Type: application/json" \
  -d '{"key_id":1,"title":"Где находится 305 аудитория?","description":"Подскажите корпус"}'
echo -e "\n"

echo "=== 6. Модератор отвечает на вопрос (id=1) ==="
curl -s -i -X POST http://localhost:8080/api/v1/qa/1/answer \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"answer":"На 3 этаже Главного корпуса"}'
echo -e "\n"

echo "=== 7. Админ одобряет и публикует вопрос (id=1) ==="
curl -s -i -X POST http://localhost:8080/api/v1/qa/1/approve \
  -H "Authorization: Bearer $ACCESS_TOKEN"
echo -e "\n"

echo "=== 8. Проверяем уведомления администратора ==="
curl -s -i -X GET http://localhost:8080/api/v1/notifications \
  -H "Authorization: Bearer $ACCESS_TOKEN"
echo -e "\n"

echo "=== Сквозной тест успешно завершен! ==="
```

---

## Вариант 2. Windows (PowerShell + curl.exe)

Запустите данный скрипт в PowerShell. Обратите внимание, что используется встроенная утилита `curl.exe` и экранирование кавычек для передачи JSON.

```powershell
# Очистка и установка переменных
$baseUrl = "http://localhost:8080"

Write-Host "=== 1. Логин суперадмином и получение ACCESS_TOKEN ===" -ForegroundColor Cyan
$loginBody = '{"login": "admin", "password": "admin123"}'
$response = Invoke-RestMethod -Uri "$baseUrl/api/v1/auth/login" -Method Post -Body $loginBody -ContentType "application/json"
$env:ACCESS_TOKEN = $response.access_token

Write-Host "Полученный Token: $env:ACCESS_TOKEN" -ForegroundColor Green

Write-Host "=== 2. Создаём аудиторию (получит id=1) ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/rooms" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"number":"305","floor":3,"building":"Главный","longitude":44.68,"latitude":43.02,"description":"Лекционная"}'

Write-Host "`n=== 3. Создаём преподавателя с привязкой к аудитории id=1 ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/teachers" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"full_name":"Иванов Иван Иванович","department":"ИВТ","position":"Доцент","room_id":1}'

Write-Host "`n=== 4. Создаём отзыв ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/reviews" `
  -H "Content-Type: application/json" `
  -d '{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"Отличная аудитория","tags":["map"]}'

Write-Host "`n=== 5. Задаём вопрос как гость (Q&A) ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/qa" `
  -H "Content-Type: application/json" `
  -d '{"key_id":1,"title":"Где находится 305 аудитория?","description":"Подскажите корпус"}'

Write-Host "`n=== 6. Модератор отвечает на вопрос (id=1) ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/qa/1/answer" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"answer":"На 3 этаже Главного корпуса"}'

Write-Host "`n=== 7. Админ одобряет и публикует вопрос (id=1) ===" -ForegroundColor Cyan
curl.exe -i -X POST "$baseUrl/api/v1/qa/1/approve" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"

Write-Host "`n=== 8. Проверяем уведомления администратора ===" -ForegroundColor Cyan
curl.exe -i -X GET "$baseUrl/api/v1/notifications" `
  -H "Authorization: Bearer $env:ACCESS_TOKEN"

Write-Host "`n=== Сквозной тест успешно завершен! ===" -ForegroundColor Green
```