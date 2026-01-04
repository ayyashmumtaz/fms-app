# FMS Reporting (Go + HTMX + PostgreSQL)

A simple admin reporting app:
- Admin inputs raw data (records)
- App shows table and recap (totals) per period

## 1) Setup PostgreSQL
Example SQL:
```sql
CREATE USER fms_user WITH PASSWORD 'fms_password';
CREATE DATABASE fms_db OWNER fms_user;
GRANT ALL PRIVILEGES ON DATABASE fms_db TO fms_user;
```

## 2) Configure env
```bash
cp .env.example .env
```
Edit `DATABASE_URL`.

## 3) Run
```bash
go mod tidy
go run .
```

Open:
- http://localhost:8080

## 4) Notes
- App auto-creates table `fms_records` if not exists (basic migration).
- Rekap is computed by SQL (no recap table).
