# Epoch School Connect

School communication and management system by **Epoch** (Gongawela Bus Station Complex, Matale · 076 843 7970).
It sends parents, students and staff in-app notifications, runs the day-to-day school records behind them, and has a built-in AI assistant powered by Claude.

It is a single Go binary: the HTML/CSS/JS frontend is embedded, the data is in SQLite, and nothing else needs to be installed.

## Features

| Area | What it does |
|---|---|
| **Communication** | Announcements and circulars to everyone, all parents, staff, a class, a bus route or one person, as in-app notifications with a live preview |
| **Automated alerts** | Absent and late alerts (optional "present" messages), daily fee reminders and overdue notices, payment receipts, homework, exam notices, published results and bus updates. You choose the channels for each alert type in Settings |
| **Two-way messaging** | Parents can chat with teachers and admins. Parents can't message other parents |
| **Attendance** | Mark a class in one click, plus range reports with attendance percentages |
| **Fees** | Issue fees to a class or the whole school, record payments, send reminders one at a time or in bulk |
| **Homework, exams & results** | Assign homework, schedule exams, enter marks with Sri Lankan grades (A/B/C/S/W), publish results to parents |
| **Transport** | Routes, buses, drivers, and one-tap bus updates ("running late", "reached school") |
| **Reports & history** | Delivery log for every message, broken down by channel, status and category, with CSV export |
| **Parent & student app** | Mobile-style portal: today's status, attendance, fees, diary, exams, results, bus, notices, chat |
| **Epoch AI (Claude)** | Writes messages from a short description (English, Sinhala, Tamil or all three), translates, suggests chat replies, and answers questions using live school data. Parents only see data for their own children |
| **Automatic timetables** | Allocate subjects, teachers and periods per week to each class, then generate a clash-free weekly timetable in one click. Subjects are spread across the week (at most two periods a day), and single periods can be edited by hand with clash checking |
| **Teacher attendance** | Staff check in and out from "My Day" (arrivals after a set time are marked late), or the office marks the register. The dashboard shows who is present, late, absent or on leave |
| **Leave approval** | Staff apply for leave; the principal, vice principal or head of section approves or rejects it with a note. The staff member is notified either way |
| **Automatic relief teachers** | When a teacher is on approved leave or marked absent, each of their periods goes to a teacher who is free that period. Teachers of the same subject come first, then those with the lightest day. Relief teachers are notified, and parents see the relief teacher in the app's timetable |
| **Sections & classes** | Group classes into sections (Primary, Middle, Upper, A/L), each with a head of section and every class with a class teacher |
| **Admissions** | Enquiry → admission test → offer → enrolment. Enrolling creates the student and the parent login and sends a welcome message |
| **Library** | Books and copies, issue and return, overdue lists and reminders to parents |
| **Roles & security** | 13 roles, each with its own permissions (below). Teachers only see and act on their own classes. bcrypt passwords, HttpOnly session cookies, CSRF protection, failed-login limiting |

## Languages: English, සිංහල, தமிழ்

- **Screens:** every screen can be switched between English, Sinhala and Tamil, from the sign-in page, the staff top bar, or Profile in the parent app. The choice is saved to the person's account, so it follows them to any device.
- **Messages:** each person's language also decides which language their app notifications arrive in. Attendance, fee, homework, exam, result, bus, leave, relief-duty, welcome and library messages are all sent in each person's own language automatically.
- **Announcements:** when Epoch AI is enabled, tick **Translate for each recipient** and every parent gets the announcement in their own language.
- **Setting a parent's language:** set it when adding them (Users & Roles, or "Parent's language" when adding a student). Parents can also change it themselves in the app.
- **Correcting translations:** screen text is in `web/js/i18n.js` and message text in `i18n.go`. Have a native speaker review both before launch. Untranslated text falls back to English.

## Pickup changes & progress reports

- **Pickup changes:** in the app, parents tell the school when someone else is collecting their child (with that person's name, relationship and phone), when the child isn't taking the bus, or when they'll collect early. The class teacher, head of section, front office and principal are notified at once. Staff see the day's list on **Pickup Changes** (with a badge for ones waiting), confirm or decline, and the parent gets the reply in the app, in their own language.
- **Progress reports:** each child gets a plain-language report covering:
  - attendance for the last 30 days compared with the 30 before
  - each subject against the class average and the child's previous result
  - strengths and subjects needing support
  - teacher remarks and upcoming homework
  - 2–3 next steps the family can take at home

  Parents open it from **Progress** in the app; staff open it from the trophy button on the Students page. The report is in the reader's language. With Epoch AI enabled, **Write with Epoch AI** produces a warmer written summary, saved once per child, language and day to keep costs down.

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

| Role | Email | Can do |
|---|---|---|
| System Admin | `admin@epoch.lk` | Everything, including settings, integrations and the database |
| Director | `director@epoch.lk` | View-only dashboard, fees and reports; approve leave; school-wide messages |
| Principal | `principal@epoch.lk` | Everything except system settings |
| Vice Principal | `viceprincipal@epoch.lk` | Like the principal, but no user accounts or fee editing |
| Head of Section | `primaryhead@epoch.lk`, `middlehead@epoch.lk` | Their section's classes, attendance, academics, teacher attendance, leave approval, relief cover |
| Class Teacher | `nirmala@epoch.lk`, `kasun@epoch.lk`, `rizna@epoch.lk`, `tharushi@epoch.lk` | Attendance for their own class; homework, exams and messages for classes they teach |
| Subject Teacher | `ruwan@epoch.lk`, `nadeesha@epoch.lk`, `imran@epoch.lk`, `chamila@epoch.lk` | Homework, exams and messages for classes they teach |
| Accountant | `accountant@epoch.lk` | Fees, payments, reminders, reports |
| Admissions Officer | `admissions@epoch.lk` | Enquiries and enrolment |
| Front Office | `frontoffice@epoch.lk` | Students and parents, circulars, bus updates, staff register |
| Librarian | `librarian@epoch.lk` | Library |
| Parent | `parent@epoch.lk` (child: Aarav Sharma) and others under Users | Parent app |
| Student | `student@epoch.lk` | Student app |

Every staff member also has **My Day** (check in/out, today's periods, relief duties, their weekly timetable and leave), **Leave Requests**, **Substitutions**, **Timetable**, **Messages** and the **AI Assistant**.

Upgrading an existing database is automatic: the old "teacher" role becomes Class Teacher (if they have a class) or Subject Teacher. To get the full new demo school instead, delete the database file and restart with `SEED_DEMO=true`.

For a real school, set `SEED_DEMO=false` and a strong `ADMIN_PASSWORD` **before the first run**. Then add classes, teachers, students and parents from the admin console. You can create each parent's login in the same step as adding their child.

## Configuration

All settings are environment variables (or a `.env` file). See [.env.example](.env.example).

### AI

Set `ANTHROPIC_API_KEY` to enable Epoch AI. The default model is `claude-opus-5-5` (change it with `AI_MODEL`).
Requests opt into Anthropic's server-side refusal fallback (`fallbacks: "default"`), so a falsely declined request is retried on a fallback model instead of failing.

### Notifications

The system sends **in-app notifications only**: every alert, reminder, announcement and reply appears in the app (bell icon for staff, Notices in the parent app). SMS and WhatsApp sending is switched off in `notify.go` (`validChannels`), and the old provider code is kept but unused. The `SMS_*`, `WHATSAPP_*`, `WAAPI_*` and `TWILIO_*` settings in `.env` are ignored.

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
notify.go            notification engine (in-app only)
ai.go                Claude integration (compose, translate, reply, tool-using assistant)
roles.go             roles, permissions and per-class scoping
timetable.go         subjects, allocation, timetable generator, sections
staff.go             teacher attendance, leave, relief-teacher assignment, My Day
office.go            admissions and library
web/                 embedded frontend (index.html, css/, js/, assets/)
dev.sh               run from source for testing
```

---
Epoch · *Empowering possibilities through technology* · epochadminlk@gmail.com
