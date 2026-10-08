# Epoch School Connect

School communication and management system by **Epoch** (Gongawela Bus Station Complex, Matale · 076 843 7970).
It sends parents WhatsApp, SMS and in-app notifications, runs the day-to-day school records behind them, and has a built-in AI assistant powered by Claude.

It is a single Go binary: the HTML/CSS/JS frontend is embedded, the data is in SQLite, and nothing else needs to be installed.

## Features

| Area | What it does |
|---|---|
| **Communication** | Announcements and circulars to everyone, all parents, staff, a class, a bus route or one person, over App, SMS and WhatsApp, with live previews and SMS part counting |
| **Automated alerts** | Absent and late alerts (optional "present" messages), daily fee reminders and overdue notices, payment receipts, homework, exam notices, published results and bus updates. You choose the channels for each alert type in Settings |
| **Two-way messaging** | Parents can chat with teachers and admins. Parents can't message other parents |
| **Attendance** | Mark a class in one click, plus range reports with attendance percentages |
| **Fees** | Issue fees to a class or the whole school, record payments, send reminders one at a time or in bulk |
| **Homework, exams & results** | Assign homework, schedule exams, enter marks with Sri Lankan grades (A/B/C/S/W), publish results to parents |
| **Transport** | Routes, buses, drivers, and one-tap bus updates ("running late", "reached school") |
| **Reports & history** | Delivery log for every message, broken down by channel, status and category, with CSV export |
| **Parent & student app** | Mobile-style portal: today's status, attendance, fees, diary, exams, results, bus, notices, chat |
| **Epoch AI (Claude)** | Writes messages from a short description (English, Sinhala, Tamil or all three), translates, suggests chat replies, and answers questions using live school data. Parents only see data for their own children |
| **Roles & security** | Admin, teacher, parent and student roles; bcrypt passwords; HttpOnly session cookies; CSRF protection; login rate limiting |

## Quick start

### Testing (no build)

```bash
./dev.sh
```

This runs the system straight from source with `go run`, serves the UI from `web/` (HTML/CSS/JS changes show on refresh), and keeps its own test database in `.dev-data/`, so testing never touches the real school data. Then open http://localhost:8080.

### Production

```bash
cp .env.example .env        # then edit it
go build -o epoch-school .
./epoch-school
```

Go 1.27+ is required (see `go.mod`).

### Demo logins (when `SEED_DEMO=true`)

All demo accounts use the password `epoch123`, and so does the admin unless you set `ADMIN_PASSWORD`. **Change them before going live.**

| Role | Email |
|---|---|
| Admin | `admin@epoch.lk` |
| Teacher | `nirmala@epoch.lk`, `kasun@epoch.lk`, `rizna@epoch.lk` |
| Parent | `parent@epoch.lk` (child: Aarav Sharma) and others under Users |
| Student | `student@epoch.lk` |

For a real school, set `SEED_DEMO=false` and a strong `ADMIN_PASSWORD` **before the first run**. Then add classes, teachers, students and parents from the admin console. You can create each parent's login in the same step as adding their child.

## Configuration

All settings are environment variables (or a `.env` file). See [.env.example](.env.example).

### AI

Set `ANTHROPIC_API_KEY` to enable Epoch AI. The default model is `claude-opus-5-5` (change it with `AI_MODEL`).
Requests opt into Anthropic's server-side refusal fallback (`fallbacks: "default"`), so a falsely declined request is retried on a fallback model instead of failing.

### SMS providers (`SMS_PROVIDER`)

- `log` (default): simulation mode. Messages are only logged and recorded as *simulated*.
- `notifylk`: [Notify.lk](https://notify.lk) for Sri Lankan numbers.
- `twilio`: Twilio Programmable SMS.
- `webhook`: POSTs `{"to","message"}` JSON to any gateway, e.g. Dialog, Mobitel or your own relay.

### WhatsApp providers (`WHATSAPP_PROVIDER`)

- `log` (default): simulation mode.
- `meta`: WhatsApp Business Cloud API. WhatsApp only allows business-initiated messages through an **approved template**. Create a template with a single `{{1}}` body variable and set `WHATSAPP_TEMPLATE`.
- `twilio`: Twilio WhatsApp sender.

Local numbers such as `077 123 4567` become `94771234567` automatically (`DEFAULT_COUNTRY_CODE`).
In Settings, use **Send test** to check a provider.

## Database & backups

The database is kept **outside the project folder**, by default in `~/.local/share/epoch-school/epoch.db` on Linux (`EPOCH_DATA_DIR` changes the folder). If an older `data/epoch.db` is found in the project folder, it is copied there on first start.

From **Settings → Database** an admin can:

- **Move the database** to another file path. The data is copied and the system restarts itself in a few seconds. The old file is left in place.
- **Open an existing database file**, e.g. to restore a backup.
- **Download a backup**, or **Back up now** to a backup folder (default: `backups/` next to the database).
- Turn on **daily automatic backups** and choose how many to keep.

Backups are consistent snapshots (`VACUUM INTO`) that are safe to take while the system is running. Don't copy the live `.db` file directly, because recent changes may still be in the `-wal` file.
The chosen path is remembered in `config.json` in the data folder. Setting `DB_PATH` in the environment pins the path and disables the Settings option.

## Deployment

```bash
docker build -t epoch-school .
docker run -d -p 8080:8080 --env-file .env -v epoch-data:/app/data epoch-school
```

Put it behind HTTPS (Caddy or Nginx) and set `SECURE_COOKIE=true`.
In Docker the database lives in the `/app/data` volume. Keep any path you choose in Settings inside that volume.

## Project layout

```
main.go              server, routes, middleware
config.go            environment configuration
db.go                schema, settings, demo seed
storage.go           database location, config file, backups
auth.go              login, sessions, roles, profile
handlers_admin.go    dashboard, users, classes, routes, students, settings
handlers_academic.go attendance, fees and reminders, homework, exams and results
handlers_comm.go     announcements, notifications, messaging, reports, portal, scheduler
notify.go            notification engine and SMS/WhatsApp providers
ai.go                Claude integration (compose, translate, reply, tool-using assistant)
web/                 embedded frontend (index.html, css/, js/, assets/)
dev.sh               run from source for testing
```

---
Epoch · *Empowering possibilities through technology* · epochadminlk@gmail.com
