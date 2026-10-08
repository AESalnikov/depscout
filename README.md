# depscout

[Русская версия](README.ru.md)

CLI in Go to **check and update dependency versions** in Gradle and Maven projects.

You pass Maven repositories explicitly (`--repos` / `DEPSCOUT_REPOS`) — Artifactory, Nexus, Maven Central, or any Maven-compatible base URL. `settings.gradle` is **not** parsed.

| Mode | Behavior |
|------|----------|
| Default (dry-run) | Print a report; **do not** change files |
| `--apply` | Write OUTDATED versions back into project files |
| `--check` | Exit `1` if any OUTDATED (for CI) |

---

## Table of contents

1. [What it checks](#what-it-checks)
2. [Install](#install)
3. [Quick start](#quick-start)
4. [Flags reference](#flags-reference)
5. [Environment variables](#environment-variables)
6. [Exit codes](#exit-codes)
7. [Report: KIND and STATUS](#report-kind-and-status)
8. [How resolve works](#how-resolve-works)
9. [Usage examples](#usage-examples)
10. [Map file (`--map`)](#map-file---map)
11. [Host rewrite](#host-rewrite)
12. [Artifactory notes](#artifactory-notes)
13. [Troubleshooting](#troubleshooting)
14. [Limitations](#limitations)
15. [License](#license)

---

## What it checks

| KIND | Source |
|------|--------|
| `catalog` | `gradle/libs.versions.toml` (`[versions]` / `[libraries]` / `[plugins]`) |
| `property` | `gradle.properties` mapped via `"g:a:${prop}"` in build scripts |
| `inline` | Literal `"g:a:1.2.3"` in `build.gradle` / `.kts` |
| `plugin` | `plugins { id "…" version "…" }` (Groovy and Kotlin DSL) |
| `wrapper` | `gradle/wrapper/gradle-wrapper.properties` → `distributionUrl` |
| `pom` | `pom.xml` `<properties>` used as `${…}` versions |
| `pom-dep` | `pom.xml` dependency with a literal `<version>` |
| `pom-plugin` | `pom.xml` plugin with a literal `<version>` |

Dedup priority when the same GAV appears in several places: **catalog → pom → property → inline**.

---

## Install

**Homebrew** (macOS / Linux):

```bash
brew install --cask AESalnikov/tap/depscout
depscout --version
```

**Go**:

```bash
go install github.com/AESalnikov/depscout@latest
```

From source (Go **1.27+**):

```bash
git clone https://github.com/AESalnikov/depscout.git
cd depscout
go build -o depscout .
# or: make build
./depscout --version
./depscout -h
```

---

## Quick start

```bash
export DEPSCOUT_REPOS="https://repo1.maven.org/maven2"
# optional Basic Auth:
# export DEPSCOUT_USER=...
# export DEPSCOUT_PASSWORD=...

# dry-run report
./depscout /path/to/project

# write updates
./depscout --apply /path/to/project

# CI gate
./depscout --check /path/to/project
```

Stdout = table. Stderr = repo list, summary, dry-run / apply hint.

---

## Flags reference

| Flag | Default | Description |
|------|---------|-------------|
| `[path]` | `.` | Project root (Gradle and/or Maven) |
| `-h` / `-help` | | Help |
| `--version` | | Print depscout version and exit |
| `--repos` | `""` | Comma-separated Maven base URLs (merged with `DEPSCOUT_REPOS`) |
| `--apply` | `false` | Write OUTDATED versions into project files |
| `--check` | `false` | Exit `1` if any OUTDATED |
| `--catalog` | `gradle/libs.versions.toml` | Path to Version Catalog (relative or absolute) |
| `--map` | `""` | Extra `prop=group:artifact` mapping file |
| `--include-prerelease` | `false` | Allow SNAPSHOT / alpha / beta / RC / `*-feature*` |
| `--rewrite-host` | `""` | Host rewrite `old.host=new.host[,…]` |
| `--skip-deps` | `false` | Skip `gradle.properties` **and** inline coords |
| `--skip-catalog` | `false` | Skip Version Catalog |
| `--skip-plugins` | `false` | Skip Gradle `plugins { }` |
| `--skip-wrapper` | `false` | Skip Gradle Wrapper |
| `--skip-pom` | `false` | Skip `pom.xml` |
| `--debug` | `false` | Log HTTP/resolve steps to stderr |

`--repos` and `DEPSCOUT_REPOS` are **merged** (duplicates removed). Order is preserved: env first, then flag.

Maven base URL = repository root (same as Gradle `maven { url '…' }`), **not** a path to one artifact:

```text
https://repo.example.com/artifactory/public
https://nexus.example.com/repository/maven-public
https://repo1.maven.org/maven2
```

---

## Environment variables

| Variable | Purpose |
|----------|---------|
| `DEPSCOUT_REPOS` | Comma-separated Maven base URLs |
| `DEPSCOUT_USER` / `DEPSCOUT_PASSWORD` | HTTP Basic (preferred) |
| `ARTIFACTORY_USER` / `ARTIFACTORY_PASSWORD` | Auth fallback |
| `DEPSCOUT_REWRITE_HOST` | Same as `--rewrite-host` (`old=new[,…]`) |
| `DEPSCOUT_DEBUG` | Same as `--debug` if non-empty |

---

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | OK; with `--check` also means no OUTDATED |
| `1` | OUTDATED present (`--check` only) |
| `2` | Config / parse / startup / fatal error |

Ctrl+C cancels in-flight HTTP (15s client timeout per request).

---

## Report: KIND and STATUS

| STATUS | Meaning |
|--------|---------|
| `OK` | Latest is not strictly newer than current |
| `OUTDATED` | Repository has a newer **release** (unless `--include-prerelease`) |
| `SKIP` | Version-like property with no GAV mapping |
| `NOT_FOUND` | Artifact missing or no usable release versions |
| `ERROR` | Network / auth / HTTP failure (details often in COORDINATE / LATEST) |

Example:

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

## How resolve works

For each GAV, repositories are tried **in order**; first non-empty latest wins.

1. `maven-metadata.xml`
2. Directory listing (HTML Index; Artifactory JSON `children` if present)
3. If still no releases and URL looks like `…/artifactory/{repoKey}` →  
   `GET /api/search/latestVersion?g=&a=&repos=` (plain text)
4. For Gradle **plugin markers** (`*.gradle.plugin`), if still missing and current version is known:  
   read marker POM → implementation coordinates → resolve those → accept only if marker POM exists for that version

Default “release” versions match: `^[0-9]+(\.[0-9]+)*$`  
(`1.2.3` yes; `1.2.3-SNAPSHOT` / `1.2.3-feature…` no, unless `--include-prerelease`).

`--apply` updates only the files that own the version (catalog keys, properties, plugin lines, pom properties/literals, wrapper URL). It does **not** validate major-version compatibility.

---

## Usage examples

### Dry-run / apply / check

```bash
export DEPSCOUT_REPOS="https://repo.example.com/artifactory/public"

./depscout /path/to/service
./depscout --apply /path/to/service
./depscout --check /path/to/service
```

### Repos via flag (merged with env)

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

### Only Version Catalog

```bash
./depscout --skip-deps --skip-plugins --skip-wrapper --skip-pom .
```

### Only pom.xml

```bash
./depscout --skip-deps --skip-catalog --skip-plugins --skip-wrapper .
```

### Only plugins + wrapper

```bash
./depscout --skip-deps --skip-catalog --skip-pom .
```

### Only wrapper (no `--repos` required)

```bash
./depscout --skip-deps --skip-catalog --skip-plugins --skip-pom .
```

### Custom catalog path

```bash
./depscout --catalog path/to/libs.versions.toml .
```

### Include SNAPSHOT / prerelease

```bash
./depscout --include-prerelease --repos "$DEPSCOUT_REPOS" .
```

### Debug resolve (stderr)

```bash
./depscout --debug --repos "https://host/artifactory/public" .
# or: export DEPSCOUT_DEBUG=1
```

### CI snippet

```bash
export DEPSCOUT_REPOS="$MAVEN_REPO_URL"
export DEPSCOUT_USER="$CI_USER"
export DEPSCOUT_PASSWORD="$CI_PASSWORD"
depscout --check "$CI_PROJECT_DIR"
```

---

## Map file (`--map`)

Version-like keys in `gradle.properties` that never appear as `"g:a:${key}"` get `SKIP`. Map them explicitly:

```bash
cat > depscout.map <<'EOF'
# comment
tomcat.version=org.apache.tomcat.embed:tomcat-embed-core
jackson-bom.version=tools.jackson:jackson-bom
EOF

./depscout --map depscout.map --apply .
```

Format: one `property=group:artifact` per line; `#` starts a comment.  
`--map` does **not** override mappings already found in build scripts.

---

## Host rewrite

If `distributionUrl` (or repo URLs) use a hostname you cannot reach:

```bash
./depscout --rewrite-host old.example.com=new.example.net .
# or
export DEPSCOUT_REWRITE_HOST="old.example.com=new.example.net"
```

Applies to wrapper URL and to repository URLs after merge.

---

## Artifactory notes

For bases like `https://host/artifactory/public`:

- Stale `maven-metadata.xml` is common; depscout falls back to HTML/JSON index and `/api/search/latestVersion`.
- Corporate Gradle plugins often publish broken marker metadata; marker → implementation POM fallback covers many cases.

Manual checks:

```bash
curl -u "$DEPSCOUT_USER:$DEPSCOUT_PASSWORD" \
  "$REPO/com/example/lib/maven-metadata.xml"

curl -sS -u "$DEPSCOUT_USER:$DEPSCOUT_PASSWORD" \
  "$HOST/artifactory/api/search/latestVersion?g=GROUP&a=ARTIFACT&repos=public"
```

---

## Troubleshooting

| Symptom | What to try |
|---------|-------------|
| `no Maven repositories configured` | Set `--repos` or `DEPSCOUT_REPOS` |
| `not a Gradle/Maven project` | Need `build.gradle` / `.kts` and/or `pom.xml` in the root |
| Many `ERROR` / timeouts | Wrong URL, VPN/DNS, or missing auth; use `--debug` |
| Plugin `NOT_FOUND` | Include the plugin repo; try `--debug`; check marker / `latestVersion` |
| Hang | HTTP timeout is 15s per call; `--debug` shows the URL; Ctrl+C cancels |
| Build breaks after `--apply` | depscout does not check compatibility — revert with git and update selectively |

---

## Limitations

- No named Gradle args (`group:`, `name:`, `version:`) and no Version Catalog bundles
- No network resolution of parent POM / BOM imports — only what is in the local `pom.xml`
- Does not update `gradlew` / `gradle-wrapper.jar` — only `distributionUrl`
- Does not read `settings.gradle` for repositories

---

## License

MIT
