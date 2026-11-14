# Architecture

One binary, `cmd/syncer`. Everything else lives in `internal/`, one package per job. A package may import only the ones below it in this table.

| Package | Job |
|---|---|
| `cmd/syncer` | Flags, starts the interface |
| `internal/tui` | The screens. `app.go` routes between them, `ui.go` draws rows, one file per screen |
| `internal/demo` | Made-up Drive data the interface runs on |
| `internal/drive` | Read-only Google Drive client: sign-in with a Service Account key, listing, download that can be continued, export of Google documents. Standard library only |
| `internal/theme` | Colour schemes |
| `internal/i18n` | Interface texts, `locales/<lang>.json` |
| `internal/filekind` | Kind of a file by its extension |
| `internal/format` | Sizes and numbers as text |

How a screen works: `Model` holds the state, `Update` turns a key into a new state, `View` returns the frame. A frame is rows; a row is cells, each with its own text and background colour. Every frame is exactly as tall and wide as the terminal.

Where a change goes:

- **A text** — add the key to every file in `locales/`. A test fails if a catalog lacks a key the screens use.
- **A colour scheme** — add it to `theme.Schemes`. A test rejects unreadable contrast.
- **A screen** — a `scr…` constant, a `view…` and a `key…` function in a file of its own, two lines in `app.go`.
- **A file kind** — `filekind`.
- **A list with sorting and filtering** — reuse `browser` in `tui/browse.go`; the trash and SKIP are both built on it.

Rules: nothing is chosen by a key that cycles blindly — options are shown as a list. Anything the user sees is tested at 40x10 and up, in every language.

==================================================

# Архітектура

Один бінарник, `cmd/syncer`. Решта лежить в `internal/`, один пакет — одна справа. Пакет може імпортувати лише ті, що нижче за нього в цій таблиці.

| Пакет | Справа |
|---|---|
| `cmd/syncer` | Прапорці, запуск інтерфейсу |
| `internal/tui` | Екрани. `app.go` перемикає між ними, `ui.go` малює рядки, на кожен екран свій файл |
| `internal/demo` | Вигадані дані Drive, на яких працює інтерфейс |
| `internal/drive` | Клієнт Google Drive лише на читання: вхід за ключем Service Account, перелік, завантаження з продовженням, експорт документів Google. Тільки стандартна бібліотека |
| `internal/theme` | Колірні схеми |
| `internal/i18n` | Тексти інтерфейсу, `locales/<мова>.json` |
| `internal/filekind` | Тип файла за розширенням |
| `internal/format` | Розміри й числа як текст |

Як працює екран: `Model` тримає стан, `Update` перетворює клавішу на новий стан, `View` повертає кадр. Кадр — це рядки; рядок — це комірки, кожна зі своїм кольором тексту і тла. Кожен кадр рівно такої висоти й ширини, як термінал.

Куди йде зміна:

- **Текст** — додати ключ у кожен файл у `locales/`. Тест падає, якщо в каталозі бракує ключа, який використовують екрани.
- **Колірна схема** — додати в `theme.Schemes`. Тест відхиляє нечитабельний контраст.
- **Екран** — константа `scr…`, функції `view…` і `key…` в окремому файлі, два рядки в `app.go`.
- **Тип файла** — `filekind`.
- **Список із сортуванням і фільтром** — використати `browser` з `tui/browse.go`; кошик і SKIP обидва збудовані на ньому.

Правила: ніщо не обирається клавішею, яка перемикає наосліп, — варіанти показуються списком. Усе, що бачить користувач, тестується від 40x10 і більше, усіма мовами.
