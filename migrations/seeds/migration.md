# Migration Seeds Commands

## Windows (PowerShell)

Jalankan seeds satu per satu dengan urutan yang benar:

```powershell
# Province
Get-Content "migrations/seeds/001_province.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# City
Get-Content "migrations/seeds/002_city.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# District
Get-Content "migrations/seeds/003_district.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# Sub District
Get-Content "migrations/seeds/004_sub_district.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# Admin
Get-Content "migrations/seeds/005_admin.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# Client
Get-Content "migrations/seeds/006_client.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# Payment Method
Get-Content "migrations/seeds/007_payment_method.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd

# Bank Code
Get-Content "migrations/seeds/008_bank_code.sql" | docker exec -i gtd-postgres psql -U gtd_user -d gtd
```

Atau jalankan semua sekaligus:

```powershell
Get-ChildItem "migrations/seeds/*.sql" -Exclude "all.sql" | Sort-Object Name | ForEach-Object { Write-Host "Running: $($_.Name)"; Get-Content $_.FullName | docker exec -i gtd-postgres psql -U gtd_user -d gtd }
```

---

## VPS / Linux (Bash)

Jalankan seeds satu per satu dengan urutan yang benar:

```bash
# Province
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/001_province.sql

# City
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/002_city.sql

# District
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/003_district.sql

# Sub District
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/004_sub_district.sql

# Admin
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/005_admin.sql

# Client
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/006_client.sql

# Payment Method
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/007_payment_method.sql

# Bank Code
docker exec -i gtd-postgres psql -U gtd_user -d gtd < migrations/seeds/008_bank_code.sql
```

Atau jalankan semua sekaligus:

```bash
for file in migrations/seeds/0*.sql; do
  echo "Running: $file"
  docker exec -i gtd-postgres psql -U gtd_user -d gtd < "$file"
done
```

---

## Troubleshooting

### Jika terjadi error "Dirty database version"

```bash
# Cek status migration
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "SELECT * FROM schema_migrations;"

# Reset dirty state
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "UPDATE schema_migrations SET dirty = false WHERE version = <VERSION>;"

# Atau hapus versi migration tertentu untuk rollback
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "DELETE FROM schema_migrations WHERE version = <VERSION>;"
```

### Verifikasi data setelah seed

```bash
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "SELECT COUNT(*) as total_provinces FROM provinces;"
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "SELECT COUNT(*) as total_cities FROM cities;"
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "SELECT COUNT(*) as total_districts FROM districts;"
docker exec -i gtd-postgres psql -U gtd_user -d gtd -c "SELECT COUNT(*) as total_sub_districts FROM sub_districts;"
```
