# vynsh

**vynsh** (винш) — кроссплатформенная консольная утилита для очистки и анализа системы.
Показывает, что реально занимает диск, что чем занято в системе, и удаляет мусор —
но только тогда, когда вы об этом попросили.

Поддерживаемые платформы: **macOS** (Intel и Apple silicon) и **Windows** (x64, arm64).
Linux собирается и работает в базовом режиме (минимум правил очистки).

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
  • vynsh clean --yes        # removes 14.7 GB of caches
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
| `vynsh tui` | Интерактивный дашборд в терминале (обзор / диск / мусор) |
| `vynsh audit` | Журнал всех удалений: что, когда, сколько байт |
| `vynsh config` | Путь и содержимое файла конфигурации, `--init` для создания |

Глобальные флаги: `--json` (машинный вывод для всех команд), `--no-color`, `-y/--yes`,
`-v/--verbose`.

## Установка

```bash
# из исходников
git clone <repo> && cd vynsh
make build            # → ./bin/vynsh
make install          # → $GOBIN/vynsh

# или напрямую
go install github.com/jaraslaukunin/vynsh/cmd/vynsh@latest
```

Кросс-сборка (macOS + Windows, amd64 + arm64):

```bash
make release          # → dist/vynsh-<os>-<arch>[.exe]
```

## Быстрый старт

```bash
vynsh report                  # общая картина: что большое, что можно освободить
vynsh scan ~/Downloads -d 3   # куда ушло место в загрузках
vynsh clean                   # только измерить мусор (ничего не удаляет)
vynsh clean --yes             # удалить безопасные кэши
vynsh usage                   # что сейчас грузит компьютер
vynsh tui                     # интерактивный дашборд
```

## Безопасность

Утилита удаляет файлы, поэтому модель безопасности — часть продукта:

* **Ничего не удаляется без `--yes`.** Команды `clean`, `rm`, `trash empty` по умолчанию
  работают как dry-run: показывают план, размеры и что именно будет удалено.
  `rm` дополнительно спрашивает подтверждение в терминале.
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
vynsh clean --only go-build-cache,gradle-cache --yes
vynsh clean --high --yes          # включить правила с высоким риском
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
по умолчанию **не** выбираются — их нужно включать явно.

### trash — корзина

```bash
vynsh trash                    # размер и количество объектов
vynsh trash empty              # dry-run: сколько освободится
vynsh trash empty --yes        # опустошить
vynsh trash add ~/old.dmg --yes  # отправить в корзину
```

macOS: `~/.Trash` плюс `.Trashes` на подключённых томах. Доступ к `~/.Trash` защищён
TCC — если видите `no access`, дайте терминалу «Полный доступ к диску»
(Системные настройки → Конфиденциальность и безопасность → Полный доступ к диску).

Windows: `SHQueryRecycleBin` / `SHEmptyRecycleBin` через `shell32.dll`, поэтому
счётчик и размер совпадают с проводником, а `trash add` работает как обычное
удаление в корзину (с возможностью отмены).

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

### tui — дашборд

Три экрана: обзор (CPU/RAM/диски/сеть/процессы), анализ диска (`s` — скан домашнего
каталога), мусор (кэши и корзина). Клавиши: `1`/`2`/`3`, `tab`, `r`, `s`, `q`.

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

## Архитектура

```
cmd/vynsh            точка входа
internal/cli         команды на cobra, рендеринг таблиц и деревьев
internal/scan        параллельный обход дерева, агрегация размеров
internal/clean       каталог правил + движок измерения и удаления
internal/del         фильтры выборочного удаления, план, выполнение
internal/trash       корзина: darwin (rename/копирование), windows (shell32)
internal/dupes       поиск дубликатов
internal/sysinfo     ресурсы: cpu/mem/disk/net/process (gopsutil)
internal/tui         дашборд на bubbletea/lipgloss
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

* Снимки диска и дифф «что выросло за неделю».
* Планировщик: автоочистка по расписанию (launchd / Task Scheduler).
* Интерактивная очистка в TUI (выбор правил галочками, удаление по Enter).
* Восстановление из корзины (`vynsh trash restore`).
* Правила для Linux (systemd, journald, snap/flatpak).
* `vynsh scan --compare old.json` — сравнение двух отчётов.
* Docker/OrbStack: предложение `docker system prune` с посчитанным объёмом.
* HTML-отчёт для «показать коллеге».

## Лицензия

MIT.
