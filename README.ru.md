# depscout

[English version](README.md)

CLI на Go для **проверки и обновления версий** зависимостей в Gradle- и Maven-проектах.

Репозитории задаёте явно (`--repos` / `DEPSCOUT_REPOS`) — Artifactory, Nexus, Maven Central или любой Maven-compatible base URL. `settings.gradle` **не читается**.

| Режим | Поведение |
|-------|-----------|
| По умолчанию (dry-run) | Печатает отчёт, **не** меняет файлы |
| `--apply` | Записывает OUTDATED обратно в файлы проекта |
| `--check` | Код выхода `1`, если есть OUTDATED (для CI) |

---

## Содержание

1. [Что проверяет](#что-проверяет)
2. [Установка](#установка)
3. [Быстрый старт](#быстрый-старт)
4. [Справочник флагов](#справочник-флагов)
5. [Переменные окружения](#переменные-окружения)
6. [Коды выхода](#коды-выхода)
7. [Отчёт: KIND и STATUS](#отчёт-kind-и-status)
8. [Как ищется latest](#как-ищется-latest)
9. [Примеры использования](#примеры-использования)
10. [Файл маппинга (`--map`)](#файл-маппинга---map)
11. [Подмена хоста](#подмена-хоста)
12. [Заметки по Artifactory](#заметки-по-artifactory)
13. [Troubleshooting](#troubleshooting)
14. [Ограничения](#ограничения)
15. [Лицензия](#лицензия)

---

## Что проверяет

| KIND | Источник |
|------|----------|
| `catalog` | `gradle/libs.versions.toml` (`[versions]` / `[libraries]` / `[plugins]`) |
| `property` | `gradle.properties` + связь через `"g:a:${prop}"` в build-скриптах |
| `inline` | Литерал `"g:a:1.2.3"` в `build.gradle` / `.kts` |
| `plugin` | `plugins { id "…" version "…" }` (Groovy и Kotlin DSL) |
| `wrapper` | `gradle/wrapper/gradle-wrapper.properties` → `distributionUrl` |
| `pom` | `pom.xml` `<properties>`, если версия через `${…}` |
| `pom-dep` | `pom.xml` dependency с литеральной `<version>` |
| `pom-plugin` | `pom.xml` plugin с литеральной `<version>` |

Если один GAV встречается в нескольких местах, приоритет: **catalog → pom → property → inline**.

---

## Установка

**Homebrew** (macOS / Linux):

```bash
brew install --cask AESalnikov/tap/depscout
depscout --version
```

**Go**:

```bash
go install github.com/AESalnikov/depscout@latest
```

Из исходников (нужен Go **1.27+**):

```bash
git clone https://github.com/AESalnikov/depscout.git
cd depscout
go build -o depscout .
# или: make build
./depscout --version
./depscout -h
```

---

## Быстрый старт

```bash
export DEPSCOUT_REPOS="https://repo1.maven.org/maven2"
# при необходимости Basic Auth:
# export DEPSCOUT_USER=...
# export DEPSCOUT_PASSWORD=...

# только отчёт
./depscout /path/to/project

# записать обновления
./depscout --apply /path/to/project

# проверка для CI
./depscout --check /path/to/project
```

Stdout — таблица. Stderr — список репозиториев, summary, подсказка dry-run / apply.

---

## Справочник флагов

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `[path]` | `.` | Корень проекта (Gradle и/или Maven) |
| `-h` / `-help` | | Справка |
| `--version` | | Версия depscout и выход |
| `--repos` | `""` | Maven base URL через запятую (объединяется с `DEPSCOUT_REPOS`) |
| `--apply` | `false` | Записать OUTDATED в файлы |
| `--check` | `false` | Exit `1`, если есть OUTDATED |
| `--catalog` | `gradle/libs.versions.toml` | Путь к Version Catalog |
| `--map` | `""` | Файл доп. маппинга `prop=group:artifact` |
| `--include-prerelease` | `false` | Учитывать SNAPSHOT / alpha / beta / RC / `*-feature*` |
| `--rewrite-host` | `""` | Подмена хоста `old.host=new.host[,…]` |
| `--skip-deps` | `false` | Не проверять `gradle.properties` **и** inline |
| `--skip-catalog` | `false` | Не проверять Version Catalog |
| `--skip-plugins` | `false` | Не проверять `plugins { }` |
| `--skip-wrapper` | `false` | Не проверять Gradle Wrapper |
| `--skip-pom` | `false` | Не проверять `pom.xml` |
| `--debug` | `false` | Лог HTTP/resolve на stderr |

`--repos` и `DEPSCOUT_REPOS` **объединяются** (дубликаты убираются): сначала env, затем флаг.

Base URL — корень репозитория (как в Gradle `maven { url '…' }`), **не** путь к одному артефакту:

```text
https://repo.example.com/artifactory/public
https://nexus.example.com/repository/maven-public
https://repo1.maven.org/maven2
```

---

## Переменные окружения

| Переменная | Назначение |
|------------|------------|
| `DEPSCOUT_REPOS` | Maven base URL через запятую |
| `DEPSCOUT_USER` / `DEPSCOUT_PASSWORD` | HTTP Basic (приоритет) |
| `ARTIFACTORY_USER` / `ARTIFACTORY_PASSWORD` | Fallback для auth |
| `DEPSCOUT_REWRITE_HOST` | Как `--rewrite-host` (`old=new[,…]`) |
| `DEPSCOUT_DEBUG` | Как `--debug`, если значение непустое |

---

## Коды выхода

| Код | Значение |
|-----|----------|
| `0` | Успех; при `--check` — нет OUTDATED |
| `1` | Есть OUTDATED (только с `--check`) |
| `2` | Ошибка конфигурации / парсинга / старта |

Ctrl+C отменяет текущие HTTP-запросы (таймаут клиента — 15s на запрос).

---

## Отчёт: KIND и STATUS

| STATUS | Смысл |
|--------|--------|
| `OK` | Latest не строго новее current |
| `OUTDATED` | В репозитории есть более новый **релиз** (без `--include-prerelease`) |
| `SKIP` | Свойство похоже на версию, но нет маппинга на GAV |
| `NOT_FOUND` | Артефакт не найден или нет подходящих релизных версий |
| `ERROR` | Сеть / auth / HTTP (детали часто в COORDINATE / LATEST) |

Пример:

```text
репозитории:
  https://repo1.maven.org/maven2

KIND      NAME                      COORDINATE / SOURCE                        CURRENT  LATEST  STATUS
catalog   orbit-core (orbitCore)    io.orbitcart.core:orbit-core                 2.4.1    2.5.0   OUTDATED
property  orbitCoreVersion          io.orbitcart.core:orbit-core                 2.4.1    2.5.0   OUTDATED
inline    com.ex:utils              build.gradle                                 1.0.0    1.1.0   OUTDATED
plugin    org.springframework.boot  org.springframework.boot:….gradle.plugin     3.3.0    3.4.1   OUTDATED
wrapper   gradle                    https://…/distributions                      8.12.1   8.13.0  OUTDATED
pom       lib.version               com.google.guava:guava                       33.0.0   33.4.0  OUTDATED

ok=0 outdated=6 skip=0 not_found=0
dry-run (передайте --apply, чтобы записать изменения)
```

---

## Как ищется latest

Для каждого GAV репозитории обходятся **по порядку**; побеждает первый непустой latest.

1. `maven-metadata.xml`
2. Listing каталога (HTML Index; при необходимости JSON `children` Artifactory)
3. Если релизов нет и URL вида `…/artifactory/{repoKey}` →  
   `GET /api/search/latestVersion?g=&a=&repos=` (text/plain)
4. Для Gradle **plugin marker** (`*.gradle.plugin`), если всё ещё пусто и известна текущая версия:  
   POM маркера → implementation GAV → resolve → принять версию, только если есть marker POM этой версии

Релиз по умолчанию: `^[0-9]+(\.[0-9]+)*$`  
(`1.2.3` — да; `1.2.3-SNAPSHOT` / feature — нет, пока не включён `--include-prerelease`).

`--apply` правит только файлы-источники версии. Совместимость major **не** проверяется.

---

## Примеры использования

### Dry-run / apply / check

```bash
export DEPSCOUT_REPOS="https://repo.example.com/artifactory/public"

./depscout /path/to/service
./depscout --apply /path/to/service
./depscout --check /path/to/service
```

### Репозитории флагом (мержится с env)

```bash
export DEPSCOUT_REPOS="https://repo1.maven.org/maven2"
./depscout --repos "https://my.artifactory/artifactory/public,https://my.artifactory/artifactory/plugins" .
```

### Auth

```bash
export DEPSCOUT_USER="ci-bot"
export DEPSCOUT_PASSWORD="***"
export DEPSCOUT_REPOS="https://repo.example.com/artifactory/public"
./depscout --check .
```

### Только Version Catalog

```bash
./depscout --skip-deps --skip-plugins --skip-wrapper --skip-pom .
```

### Только pom.xml

```bash
./depscout --skip-deps --skip-catalog --skip-plugins --skip-wrapper .
```

### Только plugins + wrapper

```bash
./depscout --skip-deps --skip-catalog --skip-pom .
```

### Только wrapper (`--repos` не обязателен)

```bash
./depscout --skip-deps --skip-catalog --skip-plugins --skip-pom .
```

### Свой путь к catalog

```bash
./depscout --catalog path/to/libs.versions.toml .
```

### Учитывать SNAPSHOT / prerelease

```bash
./depscout --include-prerelease --repos "$DEPSCOUT_REPOS" .
```

### Отладка resolve

```bash
./depscout --debug --repos "https://host/artifactory/public" .
# или: export DEPSCOUT_DEBUG=1
```

### Фрагмент для CI

```bash
export DEPSCOUT_REPOS="$MAVEN_REPO_URL"
export DEPSCOUT_USER="$CI_USER"
export DEPSCOUT_PASSWORD="$CI_PASSWORD"
depscout --check "$CI_PROJECT_DIR"
```

---

## Файл маппинга (`--map`)

Ключи из `gradle.properties`, которые нигде не встречаются как `"g:a:${key}"`, получают `SKIP`. Задайте маппинг:

```bash
cat > depscout.map <<'EOF'
# комментарий
tomcat.version=org.apache.tomcat.embed:tomcat-embed-core
jackson-bom.version=tools.jackson:jackson-bom
EOF

./depscout --map depscout.map --apply .
```

Формат: строка `свойство=group:artifact`; `#` — комментарий.  
`--map` **не** перезаписывает маппинги, уже найденные в build-скриптах.

---

## Подмена хоста

Если в `distributionUrl` (или в URL репо) другой DNS:

```bash
./depscout --rewrite-host old.example.com=new.example.net .
# или
export DEPSCOUT_REWRITE_HOST="old.example.com=new.example.net"
```

Влияет на wrapper URL и на уже собранный список репозиториев.

---

## Заметки по Artifactory

Для base вида `https://host/artifactory/public`:

- `maven-metadata.xml` часто устаревший — есть fallback на Index и `/api/search/latestVersion`;
- у корпоративных Gradle-плагинов metadata маркера нередко «битая» — помогает цепочка marker → implementation POM.

Проверка вручную:

```bash
curl -u "$DEPSCOUT_USER:$DEPSCOUT_PASSWORD" \
  "$REPO/com/example/lib/maven-metadata.xml"

curl -sS -u "$DEPSCOUT_USER:$DEPSCOUT_PASSWORD" \
  "$HOST/artifactory/api/search/latestVersion?g=GROUP&a=ARTIFACT&repos=public"
```

---

## Troubleshooting

| Симптом | Что сделать |
|---------|-------------|
| `no Maven repositories configured` | Укажите `--repos` или `DEPSCOUT_REPOS` |
| `not a Gradle/Maven project` | В корне нужны `build.gradle` / `.kts` и/или `pom.xml` |
| Много `ERROR` / timeout | Неверный URL, VPN/DNS или нет auth; смотрите `--debug` |
| Плагин `NOT_FOUND` | Добавьте plugin-репозиторий; `--debug`; проверьте marker / `latestVersion` |
| «Зависает» | Таймаут HTTP 15s; `--debug` покажет URL; Ctrl+C отменяет |
| После `--apply` ломается сборка | Совместимость не проверяется — откат через git и точечное обновление |

---

## Ограничения

- Нет named arguments Gradle (`group:`, `name:`, `version:`) и bundles Version Catalog
- Parent POM / BOM import по сети не резолвятся — только локальный `pom.xml`
- Не обновляет `gradlew` / `gradle-wrapper.jar` — только `distributionUrl`
- Не читает репозитории из `settings.gradle`

---

## Лицензия

MIT
