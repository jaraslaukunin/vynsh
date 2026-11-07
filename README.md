# vynsh

[![release](https://img.shields.io/github/v/release/jaraslaukunin/vynsh?include_prereleases&label=release&color=7c3aed)](https://github.com/jaraslaukunin/vynsh/releases)
[![license](https://img.shields.io/badge/license-MIT-7c3aed)](LICENSE)
[![platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-7c3aed)](#установка)
[![go](https://img.shields.io/github/go-mod/go-version/jaraslaukunin/vynsh?color=7c3aed)](go.mod)
[![ci](https://github.com/jaraslaukunin/vynsh/actions/workflows/ci.yml/badge.svg)](https://github.com/jaraslaukunin/vynsh/actions/workflows/ci.yml)

Консольная утилита для очистки и анализа системы. Показывает, что занимает диск и
что нагружает компьютер, и убирает мусор — но только после того, как вы увидели
план и подтвердили его.

macOS · Windows · Linux. Один статический бинарник, без зависимостей.

<img src="assets/logo-1024.png" width="220" alt="Маскот vynsh">

## Установка

```sh
# macOS и Linux
curl -fsSL https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.ps1 | iex
```

Установщик определяет ОС и архитектуру, скачивает нужный архив из
[Releases](https://github.com/jaraslaukunin/vynsh/releases), проверяет SHA-256,
кладёт `vynsh` в `PATH` и печатает версию.

| Способ | Команда |
|---|---|
| Скрипт (macOS/Linux) | `curl -fsSL …/install.sh \| sh` |
| Скрипт (Windows) | `irm …/install.ps1 \| iex` |
| Вручную | скачать архив из Releases, распаковать, положить в `PATH` |
| Из исходников | `make install` или `go install github.com/jaraslaukunin/vynsh/cmd/vynsh@latest` |

Параметры установщика: `VYNSH_VERSION=0.3.0-beta`, `VYNSH_INSTALL_DIR=~/.local/bin`,
`VYNSH_BASE_URL=<зеркало>`, `VYNSH_NO_VERIFY=1`. В PowerShell — `-Version`,
`-InstallDir`, `-NoVerify`.

Проверка целостности вручную: `shasum -a 256 -c SHA256SUMS` (macOS) или
`sha256sum -c SHA256SUMS` (Linux).

Обновление — та же команда установки. Удаление:

```sh
rm /usr/local/bin/vynsh          # или ~/.local/bin/vynsh
```

```powershell
Remove-Item "$env:LOCALAPPDATA\Programs\vynsh" -Recurse
```

## Быстрый старт

```bash
vynsh report            # общая картина: диск, память, крупные каталоги, мусор
vynsh scan ~/Downloads  # куда именно ушло место
vynsh clean             # измерить мусор и подтвердить удаление
vynsh usage             # что сейчас нагружает компьютер
vynsh tui               # интерактивный дашборд
```

## Интерфейс

```
✻ vynsh 0.3 beta  disk cleanup & system monitor                                 macbook · 22:56:46
 1 Overview   2 Cleanup   3 Disk   4 Duplicates   5 Trash
Overview                                                                                     up 3d
──────────────────────────────────────────────────────────────────────────────────────────────────
╭─────────────────────────────────────────────╮╭─────────────────────────────────────────────╮
│ CPU                                         ││ Memory                                      │
│ Apple M2 · 8 cores · 12.4%                  ││ 9.80 GB / 16.0 GB · 61.0%                   │
│ load 1.20 1.50 1.70                         ││ swap 500 MB / 2.00 GB                       │
│ █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ ││ ██████████████████████████░░░░░░░░░░░░░░░░░ │
╰─────────────────────────────────────────────╯╰─────────────────────────────────────────────╯
╭────────────────────────────────────────────────────────────────────────────────────────╮
│ Disks                                                                                  │
│ /                  ██████████████░░░░ 114 GB free of 494 GB                            │
╰────────────────────────────────────────────────────────────────────────────────────────╯
╭────────────────────────────────────────────────────────────────────────────────────────╮
│ Network                                                                                │
│ en0          ↓ 1.20 MB/s ↑ 300 KB/s                                                    │
╰────────────────────────────────────────────────────────────────────────────────────────╯
╭────────────────────────────────────────────────────────────────────────────────────────╮
│ Top processes · 412 running                                                            │
│   4 711 42.1%   512 MB    WindowServer                                                 │
╰────────────────────────────────────────────────────────────────────────────────────────╯
                                                                               ██              ██
                                                                                 ██          ██
                                                                                   ██████████
                                                                                 ██████████████
                                                                                   ██████████
                                                                                   ██  ██  ██
╭────────────────────────────────────────────────────────────────────────────────────────────────╮
│ ✓ freed 8.72 GB in 59 targets 22:56:46                                                         │
│ 1-5 views · tab/←→ switch · r refresh · s scan · q quit                                        │
╰────────────────────────────────────────────────────────────────────────────────────────────────╯
```

Маскот живёт на поле ввода: моргает, подмигивает, шевелит усиками, подпрыгивает,
перебирает лапками, пока идёт работа, и гуляет на пару шагов вправо-влево.

| Клавиши | Действие |
|---|---|
| `1`–`5`, `tab`, `←`/`→` | переключение вкладок |
| `↑`/`↓`, `space` | выбор правил очистки |
| `a` / `n` | выбрать всё безопасное / снять всё |
| `enter`, затем `y` | выполнить действие (всегда спрашивает) |
| `u` | вернуть файлы из корзины |
| `s` / `v` | сканировать домашний каталог / сохранить снимок |
| `r` / `q` | обновить / выйти |

## Команды

| Команда | Назначение |
|---|---|
| [`scan`](#scan) | анализ диска: дерево каталогов, крупные файлы, типы файлов |
| [`clean`](#clean) | очистка кэшей, логов и временных файлов по правилам |
| [`trash`](#trash) | корзина: размер, опустошение, восстановление |
| [`rm`](#rm) | выборочное удаление по возрасту, размеру, имени, типу |
| [`dupes`](#dupes) | дубликаты файлов по содержимому |
| [`snapshot`](#snapshot) | снимки диска и сравнение «что выросло» |
| [`usage`](#usage) | CPU, память, диски, сеть, процессы |
| [`report`](#report) | сводка по системе и следующие шаги |
| [`docker`](#docker) | место в Docker / OrbStack и очистка |
| [`tui`](#tui) | интерактивный дашборд |
| [`audit`](#audit) | журнал всех удалений |
| [`config`](#config) | конфигурация |
| [`logo`](#logo) | экспорт маскота в PNG |
| `version` | версия и платформа |

Глобальные флаги: `--json`, `--no-color`, `-y/--yes`, `-v/--verbose`.

### scan

```bash
vynsh scan                          # текущий каталог
vynsh scan ~/Library -d 2 -n 15     # глубина 2, топ-15 файлов
vynsh scan / --all --no-bars        # полное дерево
vynsh scan . --min-size 100MB       # скрыть мелкое
vynsh scan ~ --json > disk.json
```

Обход параллельный, симлинки не разыменовываются, системные псевдотома
пропускаются, жёсткие ссылки считаются один раз. Флаги: `--depth`, `--top`,
`--min-size`, `--exclude`, `--bars`/`--no-bars`, `--all`, `--one-filesystem`.

### clean

```bash
vynsh clean                        # измерить и спросить
vynsh clean --list                 # каталог правил
vynsh clean --category dev         # только кэши разработчика
vynsh clean --only go-build-cache,gradle-cache
vynsh clean --high                 # включить рискованные правила
vynsh clean --admin                # системные пути (нужен sudo)
```

Категории: `dev`, `browsers`, `apps`, `logs`, `system`, `temp`.
Каталоги кэшей очищаются, но не удаляются — приложения продолжают работать.

### trash

```bash
vynsh trash                        # сколько в корзине
vynsh trash empty                  # показать и спросить
vynsh trash empty --yes            # опустошить без вопросов
vynsh trash add ./old.dmg          # отправить в корзину
vynsh trash restore                # что можно вернуть
vynsh trash restore --all          # вернуть всё
```

vynsh ведёт манифест всего, что сам отправил в корзину, и умеет вернуть файлы на
исходное место. macOS: `~/.Trash` и `.Trashes` на томах. Windows: системная
корзина через shell32. Linux: freedesktop-корзина с `.trashinfo`.

### rm

```bash
vynsh rm ~/Downloads --older-than 90d
vynsh rm . --ext log --min-size 10MB
vynsh rm ./build --empty-dirs --trash
vynsh rm /tmp --newer-than 1h --limit 1000
```

Фильтры складываются по «И». Каталоги не удаляются рекурсивно: только файлы,
совпавшие с фильтром, и опустевшие каталоги с `--empty-dirs`. Флаги: `--older-than`,
`--newer-than`, `--min-size`, `--max-size`, `--name`, `--ext`, `--exclude-name`,
`--hidden`, `--depth`, `--trash`, `--limit`, `--dry-run`.

### dupes

```bash
vynsh dupes ~/Pictures
vynsh dupes . --min-size 50MB
vynsh dupes / --json > dupes.json
```

Три прохода: размер → первые 64 КиБ → полный SHA-256. Команда только показывает;
удаление дубликатов доступно на вкладке Duplicates в TUI.

### snapshot

```bash
vynsh snapshot save ~ --name before-update
vynsh snapshot list
vynsh snapshot diff before-update          # сравнить с текущим состоянием
vynsh snapshot diff monday friday --no-scan
vynsh snapshot show before-update
vynsh snapshot delete before-update
```

```
snapshot diff before → now
total: +42.3 MB · +1 file

    CHANGE   BEFORE    AFTER  PATH
──────────  ───────  ───────  ───────────
  +47.2 MB  21.0 MB  68.2 MB  videos
gone 5.24 MB  5.24 MB        —  archives
  new 307 KB        —   307 KB  __pycache__
```

### usage

```bash
vynsh usage                 # снимок
vynsh usage --watch         # обновление раз в секунду
vynsh usage --sort mem -n 15
```

### report

```bash
vynsh report
vynsh report --skip-scan    # быстро, без обхода домашнего каталога
vynsh report --depth 2 --top 10
```

### docker

```bash
vynsh docker                  # таблица как в docker system df + размер диска VM
vynsh docker prune            # удалить неиспользуемое
vynsh docker prune --all      # плюс образы без контейнеров
vynsh docker prune --volumes  # плюс неиспользуемые volume
```

Размер образа диска показывается реально занятым на диске (sparse-файлы), а не
логическим.

### tui

```bash
vynsh tui
vynsh tui --refresh 1s
```

### audit

```bash
vynsh audit                  # последние действия
vynsh audit --tail 200
vynsh audit --path
```

### config

```bash
vynsh config                 # путь и содержимое
vynsh config --init          # создать файл настроек
```

```json
{
  "excludes": ["~/Movies/archived"],
  "protected": ["~/projects/important"],
  "scan_depth": 2,
  "top": 15,
  "trash_by_default": true,
  "no_color": false
}
```

### logo

```bash
vynsh logo                                   # vynsh-logo-1024.png, прозрачный
vynsh logo --size 512 --bg "#0d0b1a"
vynsh logo --variant hop --out hop.png
vynsh logo --ascii
```

Готовые файлы: [`assets/logo-1024.png`](assets/logo-1024.png),
[`logo-1024-dark.png`](assets/logo-1024-dark.png), [`logo-512.png`](assets/logo-512.png).

## Безопасность

* План печатается всегда. В терминале vynsh спрашивает подтверждение, в скриптах
  (без TTY) работает как dry-run, пока не передан `--yes`. `--dry-run` отменяет
  удаление в любом случае.
* Защищённые пути не трогаются: `/`, `/System`, `/usr`, `/Library`, `C:\Windows`,
  `C:\Program Files`, домашний каталог и его `Desktop`/`Documents`/`Downloads`.
  Свой список — в конфиге.
* Симлинки не разыменовываются ни при сканировании, ни при удалении.
* Каталоги кэшей очищаются, но не удаляются.
* `--trash` отправляет файлы в корзину, откуда их можно вернуть.
* Каждое удаление пишется в журнал: `vynsh audit`.

## Разработка

```bash
make build        # ./bin/vynsh
make test         # go test ./...
make race         # go test -race ./...
make check        # vet + test + gofmt
make release      # 6 бинарников + .tar.gz/.zip + SHA256SUMS в dist/
```

Публикация релиза:

```bash
git tag v0.3.0-beta && git push --tags
goreleaser release --clean     # или gh release create v0.3.0-beta dist/... --prerelease
```

Структура:

```
cmd/vynsh            точка входа
internal/cli         команды, таблицы, рендеринг
internal/scan        параллельный обход дерева
internal/clean       правила очистки и движок удаления
internal/del         фильтры выборочного удаления
internal/trash       корзина и восстановление
internal/dupes       поиск дубликатов
internal/snapshot    снимки диска
internal/sysinfo     ресурсы системы
internal/tui         дашборд и маскот
internal/logo        спрайт и экспорт PNG
internal/units, style, fsutil, audit, config
```

Платформенный код изолирован build-тегами (`*_darwin.go`, `*_windows.go`,
`*_linux.go`), поэтому `GOOS=windows go build ./...` собирается без правок.

## Лицензия

[MIT](LICENSE)
