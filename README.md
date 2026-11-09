<p align="center">
  <img src=".github/logo.svg" alt="AniLiberty Indexer Logo" width="192" height="192" />
</p>

# AniLiberty Indexer

Легковесный Torznab-совместимый прокси-сервер для интеграции [AniLiberty](https://anilibria.top/) (AniLibria) API с Prowlarr, Sonarr и Radarr.

## Содержание

- [Особенности](#особенности)
- [Принцип работы](#принцип-работы)
- [Переменные окружения](#переменные-окружения)
- [Настройка шаблона названия](#настройка-шаблона-названия)
- [Определение сезонов по TVDB](#определение-сезонов-по-tvdb)
- [Запуск в Docker](#запуск-в-docker)
- [Запуск Go-бинарника](#запуск-go-бинарника)
- [Добавление и настройка в Prowlarr](#добавление-и-настройка-в-prowlarr)
- [Переименование файлов в qBittorrent](#переименование-файлов-в-qbittorrent)
- [Спецификация API](#спецификация-api)
  - [Авторизация](#авторизация)
  - [Обзор эндпоинтов](#обзор-эндпоинтов)
  - [Подробное описание эндпоинтов](#подробное-описание-эндпоинтов)

## Особенности

- **Сезоны по TVDB**: Номер сезона и нумерация серий берутся из TheTVDB по MAL ID релиза (список [Fribb/anime-lists](https://github.com/Fribb/anime-lists)), поэтому `Kimetsu no Yaiba: Yuukaku-hen` попадает в Sonarr как S03, а не S01. Поддерживается поиск по `tvdbid`.
- **Продвинутый парсинг**: Извлечение номеров сезонов и серий из названий и меток торрентов, если релиза нет в списке соответствий.
- **Гибкие шаблоны**: Настраиваемый формат заголовков раздач с автоматической очисткой от пустых скобок и артефактов.
- **Извлечение метаданных**: Распознавание года, разрешения (1080p, 720p), типа источника (BDRip, WEBRip) и кодека (x264, x265).
- **Стандарт Torznab**: Полноценная поддержка Torznab API для работы с Prowlarr, Sonarr и Radarr (поиск `search`/`tvsearch`/`movie`, RSS-лента и capabilities).
- **Высокая производительность**: Написан на Go, скомпилирован в единый легкий бинарник без внешних зависимостей.
- **Поддержка прокси**: Встроенное проксирование скачивания `.torrent` файлов и поддержка исходящих HTTP/SOCKS5 прокси.

## Принцип работы

1. **Запрос**: Принимает Torznab-запрос от Prowlarr/Sonarr/Radarr.
2. **Поиск**: Запрашивает релизы и торренты через API AniLiberty.
3. **Парсинг и адаптация**: Определяет сезон и серии по TVDB (или по названию), извлекает качество и кодек и формирует название по шаблону.
4. **Проксирование**: Перенаправляет скачивание `.torrent` файлов через эндпоинт `/download/{hash}`.

---

## Переменные окружения

| Переменная                     | По умолчанию                      | Описание                                                                                                                                                       |
| ------------------------------ | --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `HOST`                         | `0.0.0.0`                         | Адрес интерфейса, к которому привязывается сервер.                                                                                                             |
| `PORT`                         | `3649`                            | Внутренний порт, на котором работает сервер.                                                                                                                   |
| `TORRENT_TITLE_TEMPLATE`       | [Примеры](#примеры-использования) | Кастомный шаблон для формирования названия торрента для сериалов. [Настройка шаблона названия](#настройка-шаблона-названия).                                   |
| `TORRENT_MOVIE_TITLE_TEMPLATE` | [Примеры](#примеры-использования) | Кастомный шаблон для формирования названия торрента для фильмов. [Настройка шаблона названия](#настройка-шаблона-названия).                                    |
| `ANILIBERTY_API`               | `https://anilibria.top/api/v1`    | Адрес API AniLiberty.                                                                                                                                          |
| `ANILIBERTY_SITE`              | `https://anilibria.top`           | Основной адрес сайта для генерации внешних ссылок на релизы.                                                                                                   |
| `API_KEY`                      | _(пусто)_                         | Опциональный секретный ключ для авторизации клиентов (например, `my_secret_token_123`).                                                                        |
| `UPSTREAM_PROXY`               | _(пусто)_                         | Опциональный URL прокси-сервера (HTTP, HTTPS, SOCKS5) для запросов к AniLiberty API (например, `socks5://127.0.0.1:1080` или `http://proxy.local:8080`).       |
| `PROXY_DOWNLOADS`              | `false`                           | Проксировать ссылки скачивания торрентов через локальный эндпоинт `/download/{hash}` (`true`/`false`).                                                         |
| `BASE_URL`                     | _(пусто)_                         | Базовый публичный URL индексатора (например, `https://aniliberty-indexer.domain.com` или `http://192.168.1.50:3649`), используется при `PROXY_DOWNLOADS=true`. |
| `SEASON_MAPPING`               | `true`                            | Определять сезон и нумерацию серий по TVDB через MAL ID релиза (`true`/`false`). [Подробнее](#определение-сезонов-по-tvdb).                                     |
| `MAPPING_URL`                  | [Fribb/anime-lists](https://github.com/Fribb/anime-lists) | Адрес JSON-списка соответствий MAL → TVDB (`anime-list-full.json`).                                                                  |
| `MAPPING_REFRESH_HOURS`        | `24`                              | Как часто (в часах) заново скачивать список соответствий.                                                                                                      |
| `CATALOG_REFRESH_HOURS`        | `6`                               | Как часто (в часах) заново обходить каталог AniLiberty для поиска по `tvdbid`.                                                                                 |
| `DATA_DIR`                     | `data` (`/data` в Docker)         | Каталог для кэша списка соответствий. Пустое значение отключает кэш на диске.                                                                                  |
| `QBIT_URL`                     | _(пусто)_                         | Адрес WebUI qBittorrent (например, `http://qbittorrent:8080`). Включает переименование файлов серий. [Подробнее](#переименование-файлов-в-qbittorrent).           |
| `QBIT_USERNAME` / `QBIT_PASSWORD` | _(пусто)_                      | Логин и пароль WebUI qBittorrent. Можно не задавать, если для адреса индексатора авторизация в qBittorrent отключена.                                       |
| `QBIT_CATEGORY`                | _(пусто)_                         | Проверять только торренты из этой категории qBittorrent (например, `tv-sonarr`). Пустое значение — все торренты.                                               |
| `QBIT_RENAME_INTERVAL`         | `60`                              | Как часто (в секундах) проверять торренты в qBittorrent.                                                                                                       |

## Настройка шаблона названия

| Плейсхолдер           | Пример значения                     | Описание                                                                                          |
| --------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------------- |
| `{title_latin_clean}` | `Monogatari Series`                 | Латинское (ромадзи/оригинальное) название релиза без информации о сезонах и годах.                |
| `{title_latin}`       | `Monogatari Series: Second Season`  | Оригинальное латинское название релиза в сыром виде из API.                                       |
| `{title_ru}`          | `Цикл Историй: Второй Сезон`        | Русское название релиза в сыром виде из API.                                                      |
| `{season}`            | `2`                                 | Номер сезона в виде числа.                                                                        |
| `{episodes}`          | `E01-E23` или `E01`                 | Серии, извлеченные из описания торрента. Если серии не определены - пусто.                        |
| `{season_episodes}`   | `S02E01-E23`, `S02E01` или `S02` | Сезон и серии, извлеченные из описания торрента. Если серии не определены - выводит только сезон. |
| `{year}`              | `2013`                              | Год выхода релиза (если не определен - пусто).                                                    |
| `{type}`              | `WEBRip`                            | Тип источника.                                                                                    |
| `{quality}`           | `1080p`                             | Разрешение.                                                                                       |
| `{codec}`             | `x264`                              | Нормализованный кодек.                                                                            |

- _Индексатор автоматически очищает результирующее название от артефактов, возникающих из-за пустых плейсхолдеров. Конструкции вида `()`, `[]`, `{}` автоматически удаляются. Например, если `{year}` отсутствует, шаблон `({year})` превратится в пустые скобки `()` и удалится автоматически._

### Примеры использования

1. **По умолчанию**:

   ```env
   TORRENT_TITLE_TEMPLATE="[AniLiberty] {title_latin_clean} - S{season} [RUS][{type} {quality} {codec}] ({year})"
   ```

   Результат для сериала:
   `[AniLiberty] Monogatari Series - S2 [RUS][BDRip 1080p x264] (2013)`

   ```env
   TORRENT_MOVIE_TITLE_TEMPLATE="[AniLiberty] {title_latin_clean} ({year}) [RUS][{type} {quality} {codec}]"
   ```

   Результат для фильма:
   `[AniLiberty] Gekijouban Steins;Gate: Fuka Ryouiki no Deja vu (2013) [RUS][BDRip 1080p x264]`

2. **С сериями**:
   ```env
   TORRENT_TITLE_TEMPLATE="[AniLiberty] {title_latin_clean} - {season_episodes} [RUS][{type} {quality} {codec}] ({year})"
   ```
   Результат для сериала:
   `[AniLiberty] Monogatari Series - S02E01-E23 [RUS][BDRip 1080p x264] (2013)`

## Определение сезонов по TVDB

AniLiberty публикует каждый сезон отдельным релизом, и номер сезона часто не указан в названии (`Kimetsu no Yaiba: Yuukaku-hen`, `Tokyo Revengers: Santen Sensou-hen`). Sonarr же ищет и сопоставляет раздачи по сезонам TheTVDB.

Поэтому при `SEASON_MAPPING=true` индексатор:

1. Берёт MAL ID релиза из API AniLiberty (поле `mal`, при его отсутствии — `shikimori`).
2. Находит по нему сериал TVDB, сезон и смещение серий в списке [Fribb/anime-lists](https://github.com/Fribb/anime-lists). Список скачивается при старте, кэшируется в `DATA_DIR` и обновляется раз в `MAPPING_REFRESH_HOURS` часов.
3. Подставляет сезон в `{season}`/`{season_episodes}`, сдвигает номера серий для вторых частей сезонов (например, `Yozakura-san Chi no Daisakusen 2nd Season Part 2` серии `1-4` → `S02E13-E16`) и добавляет атрибуты `tvdbid`, `season`, `episode`.
4. Принимает поиск `tvsearch` с параметром `tvdbid`: находит все релизы AniLiberty этого сериала (и сезона, если он передан) через индекс каталога, который обновляется раз в `CATALOG_REFRESH_HOURS` часов.

Если релиза нет в списке или список недоступен, сезон определяется по названию, как раньше.

| Релиз AniLiberty                           | По названию | По TVDB |
| ------------------------------------------ | ----------- | ------- |
| `Kimetsu no Yaiba: Mugen Ressha-hen (TV)`  | S01         | S02     |
| `Kimetsu no Yaiba: Yuukaku-hen`            | S01         | S03     |
| `Tokyo Revengers: Santen Sensou-hen`       | S01         | S04     |
| `Diamond no Ace: Act II Second Season Part 2` | S02      | S04, серии +13 |

---

## Запуск в Docker

1. Скачайте файлы `docker-compose.yml` и `.env.example` из репозитория.
2. Создайте файл конфигурации `.env` из примера:
   ```bash
   cp .env.example .env
   ```
   _Отредактируйте параметры в файле `.env` при необходимости._
3. Запустите контейнер:
   ```bash
   docker compose up -d
   ```
   _Кэш списка соответствий сезонов сохраняется в Docker volume `aniliberty-data`, поэтому при перезапуске его не нужно скачивать заново._

### Dev-сборка из исходников

Для разработки и тестирования изменений образ собирается из локального кода через `Dockerfile.dev`. Файл `.env` необязателен, кэш списка соответствий сохраняется в `./data`.

```bash
docker compose -f docker-compose.dev.yml up -d --build
```

После правок кода повторите ту же команду, чтобы пересобрать и перезапустить контейнер. Логи и остановка:

```bash
docker compose -f docker-compose.dev.yml logs -f
docker compose -f docker-compose.dev.yml down
```

## Запуск Go-бинарника

Вы можете скачать уже готовый скомпилированный бинарник для вашей ОС (Linux, Windows, macOS) из раздела [Releases](https://github.com/EldarKliuev/aniliberty-indexer/releases/latest) или собрать его самостоятельно:

1. Соберите бинарник:
   ```bash
   go build -o aniliberty-indexer ./cmd/indexer/
   ```
2. Создайте файл конфигурации `.env` из примера:
   ```bash
   cp .env.example .env
   ```
   _Отредактируйте параметры в файле `.env` при необходимости._
3. Запустите его:
   ```bash
   ./aniliberty-indexer
   ```
4. **Опциональная настройка службы systemd (для Linux)**:
   Чтобы сервер работал в фоновом режиме и автоматически запускался при старте системы, создайте файл службы `/etc/systemd/system/aniliberty-indexer.service`:

   ```ini
   [Unit]
   Description=AniLiberty Torznab Indexer
   After=network.target

   [Service]
   Type=simple
   User=nobody
   WorkingDirectory=/opt/aniliberty-indexer
   ExecStart=/opt/aniliberty-indexer/aniliberty-indexer
   Restart=always
   EnvironmentFile=/opt/aniliberty-indexer/.env
   StateDirectory=aniliberty-indexer
   Environment=DATA_DIR=/var/lib/aniliberty-indexer

   [Install]
   WantedBy=multi-user.target
   ```

   _`StateDirectory` создаёт каталог `/var/lib/aniliberty-indexer`, доступный на запись пользователю службы. В нём хранится кэш списка соответствий сезонов, поэтому при перезапуске его не нужно скачивать заново._

   После этого выполните команды для регистрации и запуска службы:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now aniliberty-indexer
   ```

---

## Добавление и настройка в Prowlarr

1. Перейдите в веб-панель Prowlarr.
2. Откройте вкладку **Settings** -> **Indexers**.
3. Нажмите **Add Indexer** и выберите **Generic Torznab** (for torrents).
4. Введите параметры:
   - **Name**: `AniLiberty`
   - **URL**: `http://<ip_or_hostname>:3649`.
   - **API Path**: `/api`
   - **API Key**: _(оставьте пустым, если `API_KEY` не задан)_.
5. Нажмите кнопку **Test**, после сохраните настройки (**Save**).

> [!NOTE]
> Prowlarr запоминает возможности индексатора (`t=caps`) при сохранении. Если индексатор был добавлен до появления поиска по `tvdbid`, откройте его и снова нажмите **Test** и **Save**, а затем выполните **Sync App Indexers**, чтобы Sonarr начал передавать `tvdbid`.

## Переименование файлов в qBittorrent

Файлы в раздачах AniLiberty пронумерованы с начала релиза (`..._(TV)_[01]_...mkv`), а индексатор отдаёт релиз в нумерации TVDB (`S02E01-E07`). Sonarr разбирает `[01]` как абсолютный номер 1, то есть S01E01, и не импортирует файлы с ошибкой `Episode 1x01 was not found in the grabbed release`.

Если задан `QBIT_URL`, индексатор раз в `QBIT_RENAME_INTERVAL` секунд проверяет торренты в qBittorrent и переименовывает файлы серий через WebUI API: `..._[01]_...` → `..._S02E01_...`. Сезон и сдвиг нумерации берутся те же, что и в названии релиза. Infohash при этом не меняется, раздача продолжается. Переименование происходит ещё во время загрузки, поэтому к моменту импорта Sonarr видит уже правильные имена.

Другие раздачи не затрагиваются:

1. Имя торрента или трекер должны указывать на AniLiberty/AniLibria (`AniLiberty`, `AniLibria`, `libria.fun`). Остальные торренты пропускаются без каких-либо запросов.
2. Infohash должен находиться в API AniLiberty (`/anime/torrents/{hash}`). Торрент, который только называется похоже, не будет переименован.
3. Фильмы и релизы первого сезона без сдвига нумерации не переименовываются: Sonarr и так разбирает их правильно.
4. Переименовываются только видео и субтитры с номером серии в квадратных скобках. Файлы, где уже есть `SxxEyy`, не трогаются.

Пример для `docker-compose.yml` (контейнеры в одной сети):

```env
QBIT_URL=http://qbittorrent:8080
QBIT_USERNAME=admin
QBIT_PASSWORD=adminadmin
QBIT_CATEGORY=tv-sonarr
```

Если релиз уже упал с ошибкой импорта до включения переименования, индексатор переименует его файлы при следующей проверке. После этого откройте в Sonarr **Activity** → **Queue** → **Manual Import**: эпизоды будут сопоставлены автоматически.

---

# Спецификация API

## Обзор эндпоинтов

| Метод | Эндпоинт                             | Авторизация | Описание                                   |
| ----- | ------------------------------------ | ----------- | ------------------------------------------ |
| `GET` | `/` / `/health`                      | Нет         | Health check статус сервера                |
| `GET` | `/api` / `/torznab` / `/torznab/api` | Требуется   | Torznab API (Capabilities, Search, Browse) |
| `GET` | `/download/{hash}`                   | Требуется   | Проксирование скачивания `.torrent` файла  |

## Авторизация

Если в переменной окружения `API_KEY` задан секретный ключ, авторизация становится обязательной для всех эндпоинтов, кроме `GET /` и `GET /health`.

Поддерживается 3 стандартных способа передачи ключа:

| Способ                   | Метод передачи                          | Пример                                  |
| ------------------------ | --------------------------------------- | --------------------------------------- |
| **Query Parameter**      | URL-параметр `apikey`                   | `GET /api?apikey=your_secret_key`       |
| **HTTP Header**          | Заголовок `X-Api-Key`                   | `X-Api-Key: your_secret_key`            |
| **Authorization Header** | `Authorization: Bearer` (или raw token) | `Authorization: Bearer your_secret_key` |

> [!WARNING]
> Если авторизация включена, а ключ не передан или недействителен, сервер вернет ответ `401 Unauthorized` в формате Torznab XML: `<error code="100" description="Incorrect user credentials"/>`.

---

## Подробное описание эндпоинтов

### 1. Health Check

Проверка работоспособности и доступности сервиса.

```http
GET /health
GET /
```

- **Авторизация**: Не требуется
- **Ответ (`200 OK`)**:

  ```http
  HTTP/1.1 200 OK
  Content-Type: application/json

  {"status":"ok","name":"AniLiberty Indexer"}
  ```

### 2. Torznab Capabilities

Возвращает поддерживаемые категории, типы поиска и ограничения индексатора.

```http
GET /api?t=caps
```

_(Поддерживаются также алиасы: `/torznab?t=caps`, `/torznab/api?t=caps`)_

#### Query-параметры

| Параметр | Тип      | Обязательный | Описание                                     |
| -------- | -------- | ------------ | -------------------------------------------- |
| `t`      | `string` | **Да**       | Тип операции. Фиксированное значение: `caps` |

#### Пример ответа (`200 OK`)

```http
HTTP/1.1 200 OK
Content-Type: application/xml; charset=utf-8
```

```xml
<?xml version="1.0" encoding="UTF-8"?>
<caps>
  <server version="1.0" title="AniLiberty Indexer"/>
  <limits max="50" default="25"/>
  <searching>
    <search available="yes" supportedParams="q"/>
    <tv-search available="yes" supportedParams="q,season,ep,tvdbid"/>
    <movie-search available="yes" supportedParams="q"/>
  </searching>
  <categories>
    <category id="2000" name="Movies">
      <subcat id="2030" name="Movies/SD"/>
      <subcat id="2040" name="Movies/HD"/>
      <subcat id="2045" name="Movies/UHD"/>
      <subcat id="2050" name="Movies/BluRay"/>
      <subcat id="2070" name="Movies/Anime"/>
    </category>
    <category id="5000" name="TV">
      <subcat id="5030" name="TV/SD"/>
      <subcat id="5040" name="TV/HD"/>
      <subcat id="5045" name="TV/UHD"/>
      <subcat id="5070" name="TV/Anime"/>
    </category>
  </categories>
</caps>
```

### 3. Torznab Search & Browse

Поиск аниме-релизов по названию или получение ленты последних добавленных раздач (Browse mode).

```http
GET /api?t={t}&q={q}&limit={limit}&offset={offset}
```

_(Поддерживаются также алиасы: `/torznab`, `/torznab/api`)_

#### Query-параметры

| Параметр | Тип       | По умолчанию | Описание                                                                                                                                                     |
| -------- | --------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `t`      | `string`  | Не задан     | Тип поиска: `search`, `tvsearch` (`tv`), `movie` (`movie-search`, `moviesearch`). Если не задан или `browse` — режим Browse                                  |
| `q`      | `string`  | Не задан     | Поисковый запрос.<br>• **Задан**: поиск по релизу в API AniLiberty с параллельной загрузкой торрентов.<br>• **Отсутствует**: возвращает ленту свежих раздач. |
| `cat`    | `string`  | Не задан     | Категории через запятую (например, `5070` или `2000,2070`)                                                                                                   |
| `season` | `integer` | Не задан     | Номер сезона для `tvsearch` (например, `1`, `2`)                                                                                                             |
| `ep`     | `integer` | Не задан     | Номер серии для `tvsearch` (например, `5`)                                                                                                                   |
| `tvdbid` | `integer` | Не задан     | ID сериала в TheTVDB для `tvsearch`. Релизы ищутся по списку соответствий, `q` при этом не обязателен (нужен `SEASON_MAPPING=true`).                           |
| `limit`  | `integer` | `25`         | Количество результатов на страницу (максимум `50`)                                                                                                           |
| `offset` | `integer` | `0`          | Количество пропускаемых записей (смещение) для пагинации                                                                                                     |

#### Пример ответа (`200 OK`)

```http
HTTP/1.1 200 OK
Content-Type: application/xml; charset=utf-8
```

```xml
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:torznab="http://torznab.com/schemas/2015/feed">
    <channel>
        <title>AniLiberty</title>
        <link>https://anilibria.top</link>
        <item>
            <title>[AniLiberty] Vinland Saga - S1 [RUS][WEBRip 1080p x265] (2019)</title>
            <guid>04241e64625a3b3fb8e2a5037d0f6bff0dd0a547</guid>
            <link>https://anilibria.top/anime/releases/release/vinland-saga</link>
            <comments>https://anilibria.top/anime/releases/release/vinland-saga</comments>
            <pubDate>Sat, 04 Jan 2020 21:00:00 UTC</pubDate>
            <size>18020309025</size>
            <description>Сага о Винланде / Vinland Saga...</description>
            <enclosure url="https://anilibria.top/api/v1/anime/torrents/04241e64625a3b3fb8e2a5037d0f6bff0dd0a547/file" length="18020309025" type="application/x-bittorrent"/>
            <torznab:attr name="magneturl" value="magnet:?xt=urn:btih:04241e64625a3b3fb8e2a5037d0f6bff0dd0a547&amp;dn=Vinland+Saga+-+AniLibria.TV+%5BWEBRip+1080p%5D&amp;xl=18020309025&amp;tr=http://tr.libria.fun:2710/announce&amp;tr=http://retracker.local/announce"/>
            <torznab:attr name="infohash" value="04241e64625a3b3fb8e2a5037d0f6bff0dd0a547"/>
            <torznab:attr name="category" value="5070"/>
            <torznab:attr name="seeders" value="20"/>
            <torznab:attr name="peers" value="24"/>
            <torznab:attr name="grabs" value="28901"/>
            <torznab:attr name="downloadvolumefactor" value="0"/>
            <torznab:attr name="uploadvolumefactor" value="1"/>
            <torznab:attr name="resolution" value="1080"/>
            <torznab:attr name="poster" value="/storage/releases/posters/8424/Pt0yaatoRpp2adNWT8CZgYkWmzyLo1kQ.webp"/>
            <torznab:attr name="year" value="2019"/>
            <torznab:attr name="tvdbid" value="359274"/>
            <torznab:attr name="season" value="1"/>
        </item>
    </channel>
</rss>
```

### 4. Скачивание .torrent файла

Проксирует скачивание `.torrent` файла с серверов AniLiberty API. Используется, когда клиент или торрент-качалка (например, qBittorrent) не имеет прямого доступа к API AniLiberty.

```http
GET /download/{hash}?pk={pk}
```

#### Path-параметры

| Параметр | Тип      | Описание                                         |
| -------- | -------- | ------------------------------------------------ |
| `hash`   | `string` | Infohash раздачи или идентификатор торрент-файла |

#### Query-параметры

| Параметр | Тип      | Обязательный | Описание                                                 |
| -------- | -------- | ------------ | -------------------------------------------------------- |
| `pk`     | `string` | Нет          | Pass-through ключ приватного доступа API _(опционально)_ |

#### Пример ответа (`200 OK`)

```http
HTTP/1.1 200 OK
Content-Type: application/x-bittorrent
Content-Disposition: attachment; filename="04241e64625a3b3fb8e2a5037d0f6bff0dd0a547.torrent"

[Binary .torrent data stream]
```
