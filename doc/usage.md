# Running Syncer

Needs Go 1.26 or newer.

```
go run ./cmd/syncer                     # the menu
go build -o syncer ./cmd/syncer         # a binary
go test ./...                           # the tests
```

Right now this is an interface prototype: it runs on demo data and touches neither Google Drive nor the disk.

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
go run ./cmd/syncer                     # меню
go build -o syncer ./cmd/syncer         # бінарник
go test ./...                           # тести
```

Зараз це прототип інтерфейсу: він працює на демо-даних і не торкається ні Google Drive, ні диска.

| Прапорець | Значення |
|---|---|
| `-lang` | `uk`, `en` |
| `-palette` | `terminal`, `far`, `ms-dos`, `word`, `matrix`, `dark-plus`, `gruvbox`, `nord`, `ice`, `violet`, `github` |
| `-theme` | `auto`, `dark`, `light` |
| `-first-run` | починає з майстра першого запуску |
| `-demo` | `no-drive`, `lost-access` — проблемні стани |

Меню: 1 вибір файлів і папок → 2 синхронізація → 3 SKIP-файли → 4 кошик → 5 налаштування.

Клавіші: `↑↓` вибір · `Enter` відкрити · цифри — одразу до пункту · `Space` обрати або переглянути · `← →` згорнути · `Esc` назад · `q` вихід із головного меню · `s` `g` `f` сортування, групування й фільтр у кошику та SKIP.
