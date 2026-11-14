# Service Account key

Syncer reads Google Drive as a Service Account: a robot account with its own email. It sees only what is shared with that email, and only for reading.

## Get the key

1. In [Google Cloud Console](https://console.cloud.google.com) create a project and enable **Google Drive API** in it.
2. Create a **Service Account** (not an API key). Give it no roles in the project.
3. Open the account → **Keys** → add a key of type **JSON**. A `.json` file is downloaded: this is the key.
4. In Google Drive share the folder you want copied with the account's email (`name@project.iam.gserviceaccount.com`) as **Viewer**.

## Use the key

The program does not import the key yet. For now it is used only by the test that checks the connection:

```
SYNCER_TEST_KEY=/path/to/key.json go test ./internal/drive -run TestRealList -v
```

It prints how many items the account sees.

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

Програма ще не вміє імпортувати ключ. Поки що його використовує лише тест, який перевіряє з'єднання:

```
SYNCER_TEST_KEY=/шлях/до/key.json go test ./internal/drive -run TestRealList -v
```

Він друкує, скільки елементів бачить акаунт.

## Безпека

- Ключ відкриває все, що розшарено на акаунт. Тримайте його поза репозиторієм; `.gitignore` уже відхиляє такі файли.
- Якщо ключ потрапив не туди — видаліть його в Google Cloud Console → акаунт → Keys. Він перестане працювати одразу.
