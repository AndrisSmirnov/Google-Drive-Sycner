# Service Account key

Syncer reads Google Drive as a Service Account: a robot account with its own email. It sees only what is shared with that email, and only for reading.

## Get the key

1. In [Google Cloud Console](https://console.cloud.google.com) create a project and enable **Google Drive API** in it.
2. Create a **Service Account** (not an API key). Give it no roles in the project.
3. Open the account → **Keys** → add a key of type **JSON**. A `.json` file is downloaded: this is the key.
4. In Google Drive share the folder you want copied with the account's email (`name@project.iam.gserviceaccount.com`) as **Viewer**.

## Use the key

```
./syncer key /path/to/key.json
```

The program checks that the file is a Service Account key, copies it to its own folder with access for you only, and prints the email to share the folder with. That folder is `~/.config/syncer` on macOS and Linux, `%AppData%\syncer` on Windows; `SYNCER_HOME` overrides it. The downloaded file is no longer needed after the import.

## Keep it safe

- The key opens everything shared with the account. Keep it outside the repository; `.gitignore` already rejects such files.
- If the key leaks, delete it in Google Cloud Console → the account → Keys. It stops working at once.

==================================================

# Ключ Service Account

Syncer читає Google Drive від імені Service Account — службового акаунта з власним email. Він бачить лише те, що розшарено на цей email, і лише на читання.

## Де взяти ключ

1. У [Google Cloud Console](https://console.cloud.google.com) створіть проєкт і ввімкніть у ньому **Google Drive API**.
2. Створіть **Service Account** (не API key). Ролей у проєкті йому не давайте.
3. Відкрийте акаунт → **Keys** → додайте ключ типу **JSON**. Завантажиться файл `.json` — це і є ключ.
4. У Google Drive розшарте папку, яку треба копіювати, на email акаунта (`назва@проєкт.iam.gserviceaccount.com`) з роллю **Читач**.

## Як підключити ключ

```
./syncer key /шлях/до/key.json
```

Програма перевіряє, що файл є ключем Service Account, копіює його у власну папку з доступом лише для вас і друкує email, на який треба розшарити папку. Ця папка — `~/.config/syncer` у macOS і Linux, `%AppData%\syncer` у Windows; `SYNCER_HOME` її змінює. Завантажений файл після імпорту більше не потрібен.

## Безпека

- Ключ відкриває все, що розшарено на акаунт. Тримайте його поза репозиторієм; `.gitignore` уже відхиляє такі файли.
- Якщо ключ потрапив не туди — видаліть його в Google Cloud Console → акаунт → Keys. Він перестане працювати одразу.
