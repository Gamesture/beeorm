# Code review backlog — Gamesture/beeorm

Baseline: fork upstream **v1.20.1**. Review z 2026-06-25 (4 niezależne przeglądy: współbieżność, SQL, obsługa błędów, jakość/API).

Format: każda pozycja to checkbox — odhaczamy w miarę walki. ✅ przy ID = bug zweryfikowany ręcznie w kodzie, reszta = ustalenie reviewera (pewność oznaczona w treści).

---

## 🔴 CRITICAL — potwierdzone bugi

- [ ] **C1 · `where.go:46`** ✅ — panic na `nil` w parametrach (`reflect.TypeOf(nil).Kind()` → nil deref) ORAZ pusty slice w `IN ?` generuje `IN ()` (niepoprawny SQL → crash). Oba trafialne z danych użytkownika.
  - Fix: na początku pętli `if value == nil { append(nil); continue }`; dla slice `if length == 0` zamień `IN ?` na `IN (NULL)`.
- [ ] **C2 · `db.go:352, 386, 447`** ✅ *(3 reviewerów niezależnie)* — fałszywa detekcja timeoutu: `_, isTimeout := ctx.Deadline()` zwraca "czy deadline USTAWIONY", nie "czy MINĄŁ". Kontekst z `WithTimeout` zawsze ma deadline → każdy błąd maskowany jako fałszywy timeout 1969, prawdziwe błędy MySQL znikają.
  - Fix: `if errors.Is(err, context.DeadlineExceeded)`.
- [ ] **C3 · `table_schema.go:362`** ✅ — walidacja redis poola sprawdza `registry.mysqlPools[redisCache]` zamiast `registry.redisPools`. Encja z nieistniejącym redis poolem przechodzi walidację → panic w runtime przy `GetRedis()`.
  - Fix: `_, has = registry.redisPools[redisCache]`.
- [ ] **C4 · `flusher.go:902`** ✅ — `lazyMap["i"] = updatesMap` zamiast `idsMap`. ID-ki mieszają się z zapytaniami w lazy flush → uszkodzone dane.
  - Fix: `lazyMap["i"] = idsMap`.
- [ ] **C5 · `background_consumer.go:214, 245, 367`** *(niezweryfikowane, wysokie prawdopodobieństwo)* — `err.(*mysql.MySQLError)` bez sprawdzenia → panic przy błędzie połączenia/kontekstu w trakcie lazy flush.
  - Fix: `var me *mysql.MySQLError; if !errors.As(err, &me) { panic(err) }`.

## 🟠 HIGH — współbieżność (wykryje `go test -race`; część cicha przy niskim ruchu)

- [ ] **`background_consumer.go:210`** — współdzielony `*DB` między goroutine w `groupQueries` (używa `r.engine.GetMysql()` zamiast `Clone()`); wyścig na `tx`. Najgroźniejsze z tej grupy.
- [ ] **`background_consumer.go` (lazyError)** *(2 reviewerów)* — `lazyError` czytane po `wg.Wait()` bez bariery; formalny race wg Go memory model. Fix: `atomic.Value` lub odczyt pod lockiem.
- [ ] **`uuid.go:17` / `orm.go` (`disableCacheHashCheck`)** — nieatomowe globalne zmienne. Fix: `sync/atomic`.
- [ ] **`locker.go:32`** — `GetLocker()` lazy-init bez synchronizacji. Fix: `sync.Once`.
- [ ] **`engine.go:68`** — `Clone()` czyta pola engine bez locka.
- [ ] **Engine nie jest goroutine-safe**, a API tego nie wymusza ani nie dokumentuje — każda goroutine musi `Clone()`. Dodać ostrzeżenie do godoc.

## 🟠 HIGH — SQL/schema (ryzyko developer-level: migracje/DDL, nie exploit od użytkownika)

- [ ] **`schema.go:77,95,222,264`** — `SHOW TABLES LIKE '%s'` bez escapowania `%`/`_`; encja `table=user_stats` może błędnie wykryć istnienie tabeli → zły `CREATE`/`ALTER` → **utrata danych**. Fix: escape wildcardów lub `INFORMATION_SCHEMA` z `=`.
- [ ] **`schema.go:510`** — `getForeignKeys` interpoluje `tableName` przez `%s`; sparametryzować (`?`).
- [ ] **`schema.go:766`** — tag `decimal` bez walidacji (`decimal=10` lub `decimal=abc,2` → panic/zły DDL). Fix: walidacja `len==2` + `Atoi` w `Registry.Validate()`.
- [ ] **`table_schema.go:284`** — brak backticków na `logTableName` w `GetEntityLogs`.
- [ ] **`flusher.go:642`** — brak backticków na `tableName` w `flushOnDuplicateKey`.
- [ ] **`pager.go:39`** — brak walidacji `CurrentPage`/`PageSize` (wartości z HTTP → `LIMIT -N,M` → crash).

> ✅ Ścieżka wartości użytkownika (WHERE) jest w pełni parametryzowana — brak SQL injection od końcowego użytkownika. `escapeSQLString` poprawny.

## 🟡 MEDIUM — wydajność i jakość

- [ ] **`bind_builder.go:794`** — `buildJSONs` robi podwójny marshal+unmarshal + `cmp.Equal` (biblioteka testowa!) w gorącej ścieżce każdego `Flush`. Porównuj bajty; wyrzuć `google/go-cmp` z prod.
- [ ] **`serializer.go:124`** — `reflect.SliceHeader`/`StringHeader` deprecated, UB w Go 1.21+. Fix: `unsafe.StringData` (wymaga Go 1.20+).
- [ ] **`bind_builder.go:196,659`** — `math.Pow10(precision)` w pętli dirty-check; prekompilować współczynnik w schemacie.
- [ ] **`redis_cache.go`** — zero `context.Context` w ~66 metodach; brak anulowania/timeoutów dla Redis.
- [ ] **`orm.go:749`** — `SetField` nie waliduje enum/set; można wstawić nielegalną wartość enuma.
- [ ] **`Kind().String() == "..."`** w 7 miejscach (`orm.go:711,1003`, `schema.go:722`, `table_schema.go:755,1024,1083`, `where.go:46`) — porównanie stringów zamiast `== reflect.Struct`.
- [ ] **`engine.go:48`** — `*DB` zwracany z `GetMysql()` ma niechroniony stan (`tx`, `queryTimeLimit`); współdzielenie wskaźnika między goroutine niebezpieczne.

## 🧪 Testy — dostosowanie do MySQL 8.4+ (decyzja: 5.7 olewamy)

> Cel projektu: **MySQL 8.4+**. Stary 5.7 nie jest wspierany.

Zrobione (2026-06-25):
- [x] Usunięta ścieżka „version 5" (port 3311) w `global_test.go` — zunifikowane na 8.x (port 3312, z `limit_connections=10`).
- [x] Usunięty `TestSchema5` (zostaje `TestSchema8`).
- [x] Komunikat duplicate key na format 8.x (`tabela.indeks`): `flusher_test.go` (3 miejsca: 615/796/813), `lazy_flush_test.go` (4 miejsca) → `flushentity.name` / `lazyreceiverentity.name`.
- [x] `TestValidatedRegistry` — major version `8`, port 3312.

Pozostało (większe / spoza prostego version-stringa):
- [x] **`TestSchema8`** — naprawione. Differ OK (zweryfikowane: wszystkie asercje liczby alterów i idempotencji `Len(alters,0)` przechodzą; beeorm generuje poprawny DDL 8.x z jawnym `COLLATE utf8mb4_0900_ai_ci` i `int unsigned`). Failowała tylko **jedna przeterminowana asercja** (`schema_test.go:290`, styl 5.7: `int(10)`, brak COLLATE) — zaktualizowana pod 8.x. NIE był to bug w kodzie.
- [x] **TestFlush\* — różnica 1h (timezone)** — naprawione: dodany `TestMain` (`main_test.go`) wymuszający `TZ=UTC` dla procesu testowego (BeeORM wymaga UTC; `go test` nie odpala `main()`).
- [ ] `docker-compose.yml` — opcjonalnie usunąć serwis `mysql_orm` (5.7), zostawić `mysql8_orm` (lub cały compose, skoro jedziemy lokalnie).
- [ ] `testSchema` — doczyścić martwe gałęzie `if version == 5` (po usunięciu `TestSchema5` nieosiągalne).

## 🟢 LOW — dług techniczny / zależności

- [ ] `go-redis/v9 v9.0.0-beta.2` (beta z 2022) → stable v9 (breaking changes w `XPending`, którego używamy).
- [ ] `json-iterator` niemaintainowany + `ConfigFastest` wyłącza HTML-escaping; rozważyć stdlib.
- [ ] `pkg/errors` w maintenance mode → stdlib `fmt.Errorf("%w")`.
- [ ] `go 1.19` → bump do 1.21+ (odblokuje `unsafe.StringData`, `slices`, `errors.Join`).
- [ ] Ignorowane błędy: `_ = jsoniter...Unmarshal` (`orm.go:714`), `GetSet` (`redis_cache.go:27`).
- [ ] Podwójne `def()`/Close na rows (`load_by_ids.go:120+140`, `search.go`).
- [ ] `FlushDB` używa `KEYS '*'` → blokuje Redis; użyć `SCAN`.
- [ ] `regexp.Compile` przy każdym błędzie w `db.go:487` → `MustCompile` na poziomie pakietu.
- [ ] `background_consumer.go:525` — pętla reload skryptu Redis może się zapętlić w nieskończoność jeśli skrypt znika po `ScriptLoad`.

---

## Pokrycie testami

- **26 plików testowych** vs 28 źródłowych; **5869 linii testów** vs 10891 źródeł (~54% wg linii).
- **Zmierzone pokrycie: 84.8% statementów** (2026-06-25, na lokalnym MySQL 8.4 + Redis 8.6, porty przekierowane socatem 3311/3312→3308, 6381/6382→6379; pominięte testy wrażliwe na wersję).
- **Faile przy uruchomieniu = dryf wersji, NIE bugi kodu**: kod v1.20.1 pisany pod MySQL 5.7/8.0 + Redis 6/7 + `go-redis v9 beta.2`. Na nowszych serwerach panikuje:
  - `go-redis beta.2` nie rozumie pola `idmp-duration` w `XINFO STREAM` od Redis 8.x → panic w `TestRedis6/7` (= znalezisko LOW: bump go-redis do stable).
  - MySQL 8.4 generuje inny schemat/komunikaty niż 5.7 → `TestSchema5`, format błędu duplicate key (`'flushentity.name'` vs `'name'`), `TestValidatedRegistry` (major version 5 vs 8).
  - **To dlatego istnieje `docker-compose.yml`** — pinuje stare wersje (MySQL 5.7/8.0, Redis 6.2/7.0), pod które testy były pisane. Bez pinów część testów nie przejdzie na nowoczesnym środowisku.
- **Najsłabiej pokryte** (0% w tym przebiegu — częściowo bo pominęliśmy testy lazy-flush/stream, które je ćwiczą): `background_consumer.go`, `event_broker.go`. To dokładnie moduły z bugami C2/C5 i wyścigami — realna luka.
- **Pliki źródłowe BEZ dedykowanego `_test.go`** (mogą być testowane pośrednio):
  - `background_consumer.go` ⚠️ — tu jest najwięcej znalezisk CRITICAL/HIGH, a brak bezpośrednich testów.
  - `bind_builder.go` ⚠️ — gorąca ścieżka budowania bindów.
  - `table_schema.go` ⚠️ — 1235 linii, rdzeń schematu.
  - `serializer.go`, `clear_by_ids.go`, `query_logger.go`.

**Wniosek:** pszczelarz pisał testy i ma ich sporo, ale najbardziej ryzykowne moduły (lazy flush / background consumer / bind builder / table schema) nie mają dedykowanego pokrycia — a to dokładnie tam siedzą bugi C1–C5. Przy naprawianiu warto najpierw dopisać test reprodukujący, potem fix (TDD).
