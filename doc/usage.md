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
./syncer -demo data                     # made-up data: reads no Drive, writes nothing
```

On a first start the menu asks for the language, the key file, and the folder to copy to. After that it remembers them, on the computer and on the drive.

| Flag | Values |
|---|---|
| `-lang` | `uk`, `en` |
| `-palette` | `terminal`, `far`, `ms-dos`, `word`, `matrix`, `dark-plus`, `gruvbox`, `nord`, `ice`, `violet`, `github` |
| `-theme` | `auto`, `dark`, `light` |
| `-demo` | `data`, or a problem state: `no-drive`, `lost-access` |
| `-first-run` | with `-demo`: start with the first-run steps |

Without a flag the language, palette and variant come from the settings.

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
./syncer -demo data                     # вигадані дані: не читає Drive, нічого не пише
```

Під час першого запуску меню питає мову, файл ключа і папку, куди копіювати. Далі воно їх пам'ятає — на комп'ютері й на носії.

| Прапорець | Значення |
|---|---|
| `-lang` | `uk`, `en` |
| `-palette` | `terminal`, `far`, `ms-dos`, `word`, `matrix`, `dark-plus`, `gruvbox`, `nord`, `ice`, `violet`, `github` |
| `-theme` | `auto`, `dark`, `light` |
| `-demo` | `data` або проблемний стан: `no-drive`, `lost-access` |
| `-first-run` | разом із `-demo`: почати з кроків першого запуску |

Без прапорця мова, палітра й варіант беруться з налаштувань.

Меню: 1 вибір файлів і папок → 2 синхронізація → 3 SKIP-файли → 4 кошик → 5 налаштування.

Клавіші: `↑↓` вибір · `Enter` відкрити · цифри — одразу до пункту · `Space` обрати або переглянути · `← →` згорнути · `Esc` назад · `q` вихід із головного меню · `s` `g` `f` сортування, групування й фільтр у кошику та SKIP.
