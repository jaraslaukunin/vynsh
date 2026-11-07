# vynsh

**vynsh** (винш) — кроссплатформенная консольная утилита для очистки и анализа системы.
Показывает, что реально занимает диск, что чем занято в системе, и удаляет мусор —
но только тогда, когда вы об этом попросили.

Поддерживаемые платформы: **macOS** (Intel и Apple silicon) и **Windows** (x64, arm64).
Linux поддерживается тем же кодом (правила очистки, корзина freedesktop
`~/.local/share/Trash`, сканер, снимки, TUI) — собранные бинарники в `dist/`.

**Текущая версия: 0.3 beta.** Go для установки не нужен — есть готовые сборки
и установка одной строкой:

```sh
# macOS и Linux
curl -fsSL https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.ps1 | iex
```

Установщик сам определит систему и процессор (macOS/Linux/Windows, Intel/Apple
silicon/ARM), скачает архив, **проверит SHA-256**, положит бинарник в `PATH` и
покажет версию. Подробности и ручная установка — в разделе [Установка](#установка).

```
$ vynsh report
vynsh report 2026-11-07 20:47
host: MacBook-Air · macOS 27.0 · darwin/arm64

disk: /                ███████████████░░░░░  75% used of 494 GB · 126 GB free
mem:  7.05 GB / 8.59 GB · 82.0%

Home ~ · 625 GB · 817 510 files
   SIZE  SHARE                    DIRECTORY
───────  ─────  ────────────────  ────────────
 544 GB  87.0%  ██████████████░░  Library
16.9 GB   2.7%  ░░░░░░░░░░░░░░░░  Downloads
...

Reclaimable 14.7 GB
   SIZE  WHAT                 RULE
───────  ───────────────────  ──────────────
8.72 GB  All user app caches  user-caches
3.29 GB  Gradle caches        gradle-cache
...

What to do next
  • vynsh clean              # removes 14.7 GB of caches
  • vynsh scan ~/Library --depth 2   # Library is 87% of your home
```

---

## Что умеет

| Команда | Что делает |
|---|---|
| `vynsh scan [путь]` | Анализ диска: дерево каталогов с размерами и долями, крупнейшие файлы, разбивка по типам файлов |
| `vynsh clean` | Очистка кэшей, логов, временных файлов по каталогу правил (с рисками: low / medium / high) |
| `vynsh trash` | Показать размер корзины, опустошить её, отправить файлы в корзину |
| `vynsh rm <пути>` | Выборочное удаление по возрасту, размеру, имени, расширению — с планом и подтверждением |
| `vynsh dupes [пути]` | Поиск дубликатов файлов по содержимому (size → 64 KiB → SHA-256) |
| `vynsh usage` | Живой снимок CPU / памяти / свопа / дисков / сети и топ процессов (`--watch`) |
| `vynsh report` | Одна сводка: диск, память, крупные каталоги, сколько можно освободить, что делать дальше |
| `vynsh snapshot` | Снимки диска: сохранить дерево, потом посмотреть, что выросло за неделю |
| `vynsh docker` | Сколько места съедает Docker / OrbStack и `docker system prune` под контролем |
| `vynsh tui` | Интерактивный дашборд: обзор, очистка галочками, диск, корзина + маскот |
| `vynsh audit` | Журнал всех удалений: что, когда, сколько байт |
| `vynsh config` | Путь и содержимое файла конфигурации, `--init` для создания |

Глобальные флаги: `--json` (машинный вывод для всех команд), `--no-color`, `-y/--yes`,
`-v/--verbose`.

## Установка

### Одной строкой (рекомендуется)

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.sh | sh
```

```powershell
# Windows
irm https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.ps1 | iex
```

Что делает установщик:

* определяет ОС и архитектуру (macOS, Linux, Windows × amd64, arm64);
* скачивает нужный архив из [Releases](https://github.com/jaraslaukunin/vynsh/releases);
* проверяет SHA-256 по файлу `SHA256SUMS`;
* ставит бинарник в `/usr/local/bin` (или `~/.local/bin`, если первый недоступен),
  на Windows — в `%LOCALAPPDATA%\Programs\vynsh` и добавляет папку в `PATH`;
* печатает установленную версию и подсказку про `PATH`, если нужно.

Полезные переменные окружения:

| Переменная | Зачем |
|---|---|
| `VYNSH_VERSION=0.3.0-beta` | поставить конкретную версию вместо последней |
| `VYNSH_INSTALL_DIR=~/.local/bin` | куда положить бинарник |
| `VYNSH_BASE_URL=...` | свой зеркало/локальная папка вместо GitHub |
| `VYNSH_NO_VERIFY=1` | пропустить проверку контрольной суммы (не рекомендуется) |

В PowerShell те же параметры: `-Version`, `-InstallDir` и `-NoVerify`.

### Проверить перед запуском

Архивы и контрольные суммы лежат в разделе Releases. Проверка вручную:

```sh
shasum -a 256 -c SHA256SUMS           # macOS
sha256sum -c SHA256SUMS               # Linux
```

Установщик использует `curl`/`wget` и `sh` — ничего больше не требует.

### Вручную (без установщика)

1. Скачайте архив `vynsh_<версия>_<os>_<arch>.tar.gz` (или `.zip` для Windows).
2. Распакуйте и положите `vynsh` туда, где лежат другие программы:

```sh
tar -xzf vynsh_0.3.0-beta_darwin_arm64.tar.gz
sudo mv vynsh /usr/local/bin/
vynsh version
```

Windows: распакуйте `.zip` и перетащите `vynsh.exe` в папку, которая есть в `PATH`
(например `%LOCALAPPDATA%\Programs\vynsh`), либо запускайте прямо из папки загрузок.

### Homebrew (в планах)

Как только появится tap, установка будет такой:

```sh
brew install jaraslaukunin/tap/vynsh
```

### Из исходников (для разработки)

```bash
git clone https://github.com/jaraslaukunin/vynsh
cd vynsh
make build            # → ./bin/vynsh
make install          # → $GOBIN
go run ./cmd/vynsh report
```

### Обновление и удаление

```sh
# обновиться до последней версии
curl -fsSL https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.sh | sh

# удалить
rm /usr/local/bin/vynsh                       # macOS / Linux
rm ~/.local/bin/vynsh
# Windows
Remove-Item "$env:LOCALAPPDATA\Programs\vynsh" -Recurse
```

Данные vynsh (конфиг, аудит-лог, снимки) лежат отдельно и удаляются вручную:
macOS — `~/Library/Application Support/vynsh`, Windows — `%AppData%\vynsh`,
Linux — `~/.config/vynsh`.

### Сборка релиза (для мейнтейнера)

```bash
make release VERSION=0.3.0-beta   # бинарники + .tar.gz/.zip + SHA256SUMS в dist/
git tag v0.3.0-beta && git push --tags
goreleaser release --clean        # то же самое + GitHub Release и changelog
```

## Быстрый старт

```bash
vynsh report                  # общая картина: что большое, что можно освободить
vynsh scan ~/Downloads -d 3   # куда ушло место в загрузках
vynsh clean                   # только измерить мусор (ничего не удаляет)
vynsh clean                   # измерить, затем подтвердить удаление
vynsh usage                   # что сейчас грузит компьютер
vynsh tui                     # интерактивный дашборд
```

## Безопасность

Утилита удаляет файлы, поэтому модель безопасности — часть продукта:

* **Ничего не удаляется молча.** Перед удалением всегда печатается план с размерами:
  в терминале `clean`, `rm` и `trash empty` спрашивают подтверждение (ответ `y`);
  в скриптах и cron (нет TTY) выполняется только dry-run, пока явно не передан `--yes`;
  `--dry-run` отменяет удаление в любом случае.
* **Защищённые пути.** Никогда не удаляются `/`, `/System`, `/usr`, `/Library`, `C:\Windows`,
  `C:\Program Files`, домашний каталог и его `Desktop`/`Documents`/`Downloads`.
  Свой список добавляется в конфиге (`protected`).
* **Символические ссылки не разыменовываются** ни при сканировании, ни при удалении.
* **Нет рекурсивного удаления по шаблону.** `vynsh rm` удаляет только файлы, совпавшие с
  фильтрами, а каталоги — только если они опустели (`--empty-dirs`). Шаблон не может
  «случайно» унести содержимое каталога, которое под фильтр не попадало.
* **Каталоги кэшей только очищаются**, а не удаляются целиком — приложения продолжают работать.
* **`--trash`** отправляет файлы в корзину ОС (на macOS — `~/.Trash`, на Windows — через
  shell API с поддержкой Undo), так что ошибка обратима.
* **Аудит.** Каждое удаление пишется в `~/.config/vynsh/audit.log` (JSON Lines),
  посмотреть: `vynsh audit`.
* **Сначала измерение, потом действие**: суммарный размер считается по реальным файлам,
  вложенные правила не считаются дважды, жёсткие ссылки учитываются один раз.

## Команды подробнее

### scan — анализ диска

```bash
vynsh scan                       # текущий каталог
vynsh scan ~/Library -d 2 -n 15  # глубина 2, топ-15 файлов
vynsh scan / --all --no-bars     # полное дерево без скрытия мелких узлов
vynsh scan . --min-size 100MB    # скрыть всё, что меньше 100 МБ
vynsh scan ~ --json > disk.json  # машинный формат
```

Обход дерева параллельный (пул воркеров по числу ядер), это в разы быстрее
однопоточного `du` на холодном кэше. По умолчанию пропускаются системные
псевдо-точки монтирования (`/System/Volumes`, `/Volumes`; на Windows —
`System Volume Information`, `$Recycle.Bin`, `WinSxS`), чтобы не считать одни и те же
данные дважды. Нечитаемые каталоги не прерывают обход — они попадают в `--json`
и в `-v` вывод.

### clean — очистка мусора

```bash
vynsh clean                       # измерить всё, ничего не удалять
vynsh clean --list                # каталог правил: id, риск, описание
vynsh clean --category dev        # только кэши разработчика
vynsh clean --only go-build-cache,gradle-cache
vynsh clean --high                # включить правила с высоким риском
vynsh clean -v --only user-caches # показать конкретные каталоги внутри правила
```

Категории: `dev`, `browsers`, `apps`, `logs`, `system`, `temp`.

macOS (примеры правил): `~/Library/Caches/*`, `~/Library/Logs/*`, Chrome/Safari/Firefox,
Xcode DerivedData, iOS/watchOS DeviceSupport, Xcode Archives, Go build & module cache,
npm/pnpm/yarn/pip/cargo/gradle/maven/Homebrew, `/Library/Caches` (нужен root).

Windows: `%TEMP%`, `%SystemRoot%\Temp`, Windows Update `SoftwareDistribution\Download`,
Delivery Optimization, Chrome/Edge/Firefox/Brave кэши, `%LOCALAPPDATA%\CrashDumps`,
NuGet, Go/npm/pnpm/yarn/pip/cargo/gradle/maven, эскизы Explorer,
`%ProgramData%\...\WER` (отчёты об ошибках).

Правила с риском `high` (архивы Xcode, iOS DeviceSupport, `~/.m2/repository`, NuGet)
по умолчанию **не** выбираются — их нужно включать явно (`--high` или `--only <id>`).

Правила, которым нужны права администратора (`/Library/Caches`, `%SystemRoot%\Temp`,
Windows Update), по умолчанию пропускаются: добавьте `--admin` и запустите через
`sudo` (или от имени администратора), чтобы включить их.

### trash — корзина

```bash
vynsh trash                    # размер и количество объектов
vynsh trash empty              # dry-run: сколько освободится
vynsh trash empty --yes        # опустошить без вопросов (для скриптов)
vynsh trash add ~/old.dmg      # отправить в корзину
```

Плюс восстановление:

```bash
vynsh trash restore            # что можно вернуть
vynsh trash restore --all      # вернуть всё (спросит подтверждение)
vynsh trash restore --id 1,3 --dry-run
```

vynsh записывает в манифест всё, что сам отправил в корзину, поэтому может вернуть
файлы на исходное место; если место занято — рядом с суффиксом. На Linux используется
freedesktop-корзина (`~/.local/share/Trash` с `.trashinfo`), на Windows — системная
корзина (её восстановление делает проводник).

macOS: `~/.Trash` плюс `.Trashes` на подключённых томах. Доступ к `~/.Trash` защищён
TCC — если видите `no access`, дайте терминалу «Полный доступ к диску»
(Системные настройки → Конфиденциальность и безопасность → Полный доступ к диску).

Windows: `SHQueryRecycleBin` / `SHEmptyRecycleBin` через `shell32.dll`, поэтому
счётчик и размер совпадают с проводником, а `trash add` работает как обычное
удаление в корзину (с возможностью отмены). Восстановление на Windows делает сам
проводник — `vynsh trash restore` там только показывает, что было отправлено в корзину
через vynsh.

### rm — выборочное удаление

```bash
vynsh rm ~/Downloads --older-than 90d
vynsh rm ~/Library/Caches --ext log --min-size 10MB
vynsh rm ./build --empty-dirs --trash
vynsh rm /tmp --newer-than 1h --limit 1000 --yes
vynsh rm ./data --name "draft-*.csv" --older-than 30d --yes
```

Фильтры объединяются по «И»: возраст (`--older-than`, `--newer-than`, форматы `30d`, `6h`, `2w`),
размер (`--min-size`, `--max-size`), имя (`--name`, glob), расширение (`--ext`),
исключения (`--exclude-name`), глубина (`--depth`), скрытые файлы (`--hidden`).
Итоговый план печатается перед удалением; `--json` отдаёт план или результат.

### dupes — дубликаты

Три прохода: группировка по размеру → хеш первых 64 KiB → полный SHA-256.
Ложных срабатываний нет, а байты читаются только там, где это нужно.
Утилита ничего не удаляет — только показывает группы и потенциальную экономию.

### snapshot — что выросло с прошлого раза

```bash
vynsh snapshot save ~ --depth 2 --name before-update   # запомнить состояние
vynsh snapshot list                                    # все снимки
vynsh snapshot diff before-update                      # сравнить с тем, что сейчас
vynsh snapshot diff monday friday --no-scan            # сравнить два снимка
vynsh snapshot show before-update                      # напечатать дерево из снимка
vynsh snapshot delete before-update
```

Снимок — это JSON с деревом каталогов, поэтому сравнение мгновенное: видно, какие
каталоги выросли, какие исчезли и на сколько изменилось число файлов. В TUI снимок
сохраняется клавишей `v` на вкладке Disk.

```
snapshot diff before → now
total: +42.3 MB · +1 file

    CHANGE   BEFORE    AFTER  PATH
──────────  ───────  ───────  ───────────
  +47.2 MB  21.0 MB  68.2 MB  videos
gone 5.24 MB  5.24 MB        —  archives
  new 307 KB        —   307 KB  __pycache__
```

### docker — место внутри контейнеров

```bash
vynsh docker                 # таблица как в `docker system df` + размер диска VM
vynsh docker prune           # удалить неиспользуемое (спросит подтверждение)
vynsh docker prune --all     # плюс образы без контейнеров
vynsh docker prune --volumes # плюс неиспользуемые volume (данные!)
```

Обычный `scan` не видит, что внутри VM-диска Docker: он показывает один большой
`Docker.raw`. `vynsh docker` показывает и логический размер, и реально занятый на диске
(файл разрежённый), а `prune` прогоняет `docker system prune` и печатает, сколько
освободилось. Если демон не запущен — подскажет, что запустить.

### usage — ресурсы

```bash
vynsh usage                 # снимок
vynsh usage --watch         # обновление раз в секунду, выход по Ctrl+C
vynsh usage --sort mem -n 15
vynsh usage --json
```

CPU в процентах от одного ядра (как в `top`), память — used/available/total, своп,
диски, скорость сети (дельта между двумя сэмплами), топ процессов по CPU или памяти.

### report — сводка

`vynsh report` собирает всё вместе. `--skip-scan` — быстро, без обхода домашнего
каталога; `--depth`/`--top` управляют детализацией. В конце — конкретные следующие шаги.

### tui — интерактивный дашборд

Четыре вкладки в одном экране, в стиле современных терминальных агентов:
заголовок, номера вкладок, панель с маскотом и подвал с подсказками по клавишам.

```
✻ vynsh  disk cleanup & system monitor                          macbook · 21:55:05
 1 Overview   2 Cleanup   3 Disk   4 Trash
Cleanup                                    8.96 GB selected · 2 rules    ╭─────────────────╮
──────────────────────────────────────────────────────────────────────   │   ╭─────╮       │
▸ [x]   8.72 GB  low    All user app caches                          59   │   │ ◕ ◕ │       │
  [ ]   3.29 GB  medium Gradle caches                                 1   │   │  ─  │       │
  [x]    238 MB  low    Go build cache                                1   │   ╰──┬──╯       │
  [ ]         —  high   Xcode archives                                    │ ╭────┴────╮     │
                                                                          │ │ ░▓░▓░▓░ │     │
                                                                          │ ╰─────────╯     │
                                                                          │                 │
                                                                          │ 8.96 GB selected│
                                                                          │ — enter? ›      │
                                                                          ╰─────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────╮
│ 8.96 GB selected — enter to clean · space to unhide…                                      │
╰──────────────────────────────────────────────────────────────────────────────────────────╯
```

**Маскот** — маленький робот, который печатает на клавиатуре: пока vynsh работает,
клавиши подсвечиваются в такт, иногда он моргает, а после успешной очистки
улыбается. Его реплика в панели подсказывает, что происходит («measuring caches…»,
«scanning your disk…», «4.2 GB in the bin»).

Вкладки и клавиши:

| Клавиши | Что делают |
|---|---|
| `1`–`4`, `tab`, `←/→` | переключение вкладок (Overview / Cleanup / Disk / Trash) |
| `↑/↓`, `space` | выбор правил очистки (размер каждого правила виден сразу) |
| `a` / `n` | отметить всё безопасное / снять всё |
| `enter`, затем `y` | выполнить очистку (всегда спрашивает) |
| `u` | вернуть файлы из корзины, `enter` — опустошить корзину |
| `s` / `v` | просканировать домашний каталог / сохранить снимок диска |
| `r` / `q` | обновить всё / выйти |

Очистка внутри TUI идёт по тем же правилам и тем же механизмам, что и `vynsh clean`
(размеры, риски, защита путей), просто вместо флагов — галочки.

## Конфигурация

`vynsh config --init` создаёт `config.json` рядом с аудит-логом
(macOS: `~/Library/Application Support/vynsh/`, Windows: `%AppData%\vynsh\`).

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

## Логотип и брендинг

Маскот — пиксельный жук, 9×6 «пикселей», один и тот же спрайт используется и в
терминале, и в логотипе:

```
L.......L
.L.....L.
.SSSSSSS.
.SEESEES.
.SHHHHHH.
..L.L.L..
```

Он сидит на верхней рамке поля ввода в TUI и иногда подпрыгивает (в прыжке
поджимает лапки), а после успешной очистки прыгает дважды.

Экспорт в PNG для айдентики:

```bash
vynsh logo                                  # vynsh-logo-1024.png, прозрачный фон
vynsh logo --size 512 --bg "#0d0b1a"        # с тёмным фоном
vynsh logo --variant hop --out hop.png      # другая поза: base, hop, blink
vynsh logo --ascii                          # посмотреть спрайт в терминале
```

Готовые файлы лежат в [`assets/`](assets/): `logo-1024.png` (прозрачный),
`logo-1024-dark.png` (тёмный фон), `logo-512.png`. Масштабирование целочисленное,
поэтому пиксели остаются резкими на любом размере.

## Архитектура

```
cmd/vynsh            точка входа
internal/cli         команды на cobra, рендеринг таблиц и деревьев
internal/scan        параллельный обход дерева, агрегация размеров
internal/clean       каталог правил + движок измерения и удаления
internal/del         фильтры выборочного удаления, план, выполнение
internal/trash       корзина: darwin (rename/копирование), windows (shell32)
internal/dupes       поиск дубликатов
internal/snapshot    снимки диска, хранение и сравнение
internal/sysinfo     ресурсы: cpu/mem/disk/net/process (gopsutil)
internal/tui         дашборд: обзор, очистка, диск, корзина, маскот
internal/units       размеры, длительности, проценты
internal/style       цвета, таблицы, прогресс (уважает NO_COLOR)
internal/fsutil      пути, защищённые каталоги, безопасное удаление, device id
internal/audit       журнал действий (JSON Lines)
internal/config      пользовательские настройки
```

Платформенные части изолированы build-тегами: `*_darwin.go`, `*_windows.go`,
`*_unix.go`, поэтому `GOOS=windows go build ./...` собирается без правок.

## Разработка

```bash
make test        # go test ./...
make race        # go test -race ./...
make vet         # go vet ./...
make fmt         # gofmt -w
make release     # кросс-сборка darwin/windows/linux × amd64/arm64
```

Тесты покрывают фильтры удаления, движок правил (включая защиту от двойного учёта
вложенных правил), сканер (глубина, симлинки, жёсткие ссылки), дубликаты,
корзину, таблицы и разбор размеров/длительностей.

## Идеи на будущее

* Планировщик: автоочистка по расписанию (launchd / Task Scheduler / systemd timer).
* `vynsh snapshot diff --watch` — следить за ростом каталога в реальном времени.
* Интерактивное удаление дубликатов в TUI (галочки прямо по группам).
* JSON-schema для конфигурации и собственные правила пользователя.
* HTML-отчёт для «показать коллеге».
* Homebrew tap и winget-манифест для установки без скриптов.

## Лицензия

MIT.
