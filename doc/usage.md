# Running Syncer

Needs Go 1.26 or newer.

```
go build -o syncer ./cmd/syncer         # a binary
go test ./...                           # the tests
```

## Sync without the menu

```
./syncer key path/to/key.json           # once: import the Service Account key
./syncer sync -dest /Volumes/USB/backup # copy what is shared with the account
./syncer sync                           # later: the folder is remembered
```

A sync prints what it is going to do, the progress, and a summary. Files deleted on Drive go into the trash (`.bin` in the destination). `Ctrl+C` stops it; the same command goes on from where it stopped. Exit code: 0 all copied, 1 some files failed or the sync was stopped, 2 it could not start.

## The menu

```
./syncer                                # or: go run ./cmd/syncer
```

The menu is still an interface prototype: it runs on demo data and touches neither Google Drive nor the disk.

| Flag | Values |
|---|---|
| `-lang` | `uk`, `en` |
| `-palette` | `terminal`, `far`, `ms-dos`, `word`, `matrix`, `dark-plus`, `gruvbox`, `nord`, `ice`, `violet`, `github` |
| `-theme` | `auto`, `dark`, `light` |
| `-first-run` | starts with the first-run wizard |
| `-demo` | `no-drive`, `lost-access` — problem states |

Menu: 1 choose files and folders → 2 sync → 3 SKIP files → 4 trash → 5 settings.

Keys: `↑↓` move · `Enter` open · digits jump to an item · `Space` select or preview · `← →` fold · `Esc` back · `q` quit from the main menu · `s` `g` `f` sort, group and filter in the trash and SKIP.

==================================================

# Запуск Syncer

Потрібен Go 1.26 або новіший.

```
go build -o syncer ./cmd/syncer         # бінарник
go test ./...                           # тести
```

## Синхронізація без меню

```
./syncer key шлях/до/key.json           # один раз: імпортувати ключ Service Account
./syncer sync -dest /Volumes/USB/backup # скопіювати те, що розшарено на акаунт
./syncer sync                           # далі: папку запам'ятовано
```

Синхронізація друкує, що збирається робити, прогрес і підсумок. Файли, видалені на Drive, ідуть у кошик (`.bin` у папці призначення). `Ctrl+C` зупиняє; та сама команда продовжує з місця зупинки. Код завершення: 0 — усе скопійовано, 1 — частина файлів не вдалася або синхронізацію зупинено, 2 — не змогла почати.

## Меню

```
./syncer                                # або: go run ./cmd/syncer
```

Меню поки що є прототипом інтерфейсу: воно працює на демо-даних і не торкається ні Google Drive, ні диска.

| Прапорець | Значення |
|---|---|
| `-lang` | `uk`, `en` |
| `-palette` | `terminal`, `far`, `ms-dos`, `word`, `matrix`, `dark-plus`, `gruvbox`, `nord`, `ice`, `violet`, `github` |
| `-theme` | `auto`, `dark`, `light` |
| `-first-run` | починає з майстра першого запуску |
| `-demo` | `no-drive`, `lost-access` — проблемні стани |

Меню: 1 вибір файлів і папок → 2 синхронізація → 3 SKIP-файли → 4 кошик → 5 налаштування.

Клавіші: `↑↓` вибір · `Enter` відкрити · цифри — одразу до пункту · `Space` обрати або переглянути · `← →` згорнути · `Esc` назад · `q` вихід із головного меню · `s` `g` `f` сортування, групування й фільтр у кошику та SKIP.
