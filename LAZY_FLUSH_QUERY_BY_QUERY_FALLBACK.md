# Lazy flush: fallback query po query (odłożone)

Branch: `lazy-flush-query-by-query-fallback` (beeorm) + `lazy-flush-query-by-query-fallback` (spring).
Stan: działa, testy zielone, **nie zmergowane i nie otagowane**. Odłożone, bo powstaje nowy ORM (moorm);
beeorm i moorm mają być porównywane równolegle na tym samym backendzie.

## Problem

`BackgroundConsumer` (kolejka `orm-lazy-channel`) dzieli UPDATE/DELETE z batcha (do 500 eventów)
na grupy `(pula, ID % lazyFlushModulo)`. Grupa z więcej niż jednym query idzie jako **jeden
multi-statement w transakcji** (`q1;q2;q3;`).

Gdy transakcja grupy padnie:

- **błąd inny niż deadlock** (np. 1062 duplicate key): rollback całej grupy, resolver dostaje
  **cały sklejony SQL grupy**. Resolver springa zwraca `nil` → ACK wszystkich eventów grupy →
  **dobre query też przepadają po cichu**;
- **deadlock**: jeden retry całej transakcji; jeśli też padnie → `// TODO report`, commit, ACK →
  **cicha utrata całej grupy**.

Przy domyślnym modulo 11 przepada ~1/11 batcha danej puli, przy `lazyFlushModulo: 1` (fishing dev)
**cały batch puli**.

## Rozwiązanie (za flagą, domyślnie wyłączone)

`SetLazyFlushQueryByQueryFallback(true)` w beeorm / `ormAsync.queryByQueryFallback: true` w YAML gry (spring).

1. Grupa najpierw jak dotąd: jedna transakcja, jeden round-trip. Zwykła ścieżka nie jest wolniejsza.
2. Gdy transakcja padnie: rollback, potem **każde query osobno w autocommit, w kolejności streamu**:
   - deadlock (1213) → retry tego query, max 3 razy (30/60/90 ms),
   - inny błąd MySQL → resolver dostaje **tylko to jedno query**; `nil` = pomiń to query i jedź dalej,
   - resolver zwraca error / błąd nie-MySQL (np. zerwane połączenie) → panic → brak ACK → batch wraca
     z kolejki (powtórka bezpieczna, lazy UPDATE ustawia wartości absolutne).
3. `log.Printf`: rozmiar grupy + błąd transakcji, potem czas fallbacku.

Bez flagi działa stary kod (identyczny z `main`, 48 linii porównane 1:1).

Koszt: po fallbacku grupa nie jest atomowa (event może zapisać się częściowo — przy modulo 11 tej
atomowości i tak nie było). Czas: pomiar z `query.log` fishing-api na forge-api-dev — zapis w autocommit
p50 3 ms / p90 5 ms, SELECT po ID ~1 ms → fallback 500 query ≈ 1,5–2,5 s, tylko na ścieżce awaryjnej.

## Testy (`lazy_flush_test.go`)

- `TestLazyFlushGroupQueryError` (flaga włączona): modulo 1, 3 lazy update, środkowy 1062 →
  resolver dostaje tylko złe query, ID 1 i 3 zapisane; bez resolvera panic, a kolejny `Digest`
  przetwarza ten sam batch (brak ACK) i kolejka jest pusta.
- `TestLazyFlushGroupQueryErrorDefault` (flaga wyłączona): dokumentuje stare zachowanie — resolver
  dostaje cały SQL grupy, wszystkie 3 zapisy przepadają.

Brak testu prawdziwego deadlocka (retry 1213 sprawdzony tylko przeglądem kodu).

## Spring (`ormAsyncCron.go`)

Czyta `ormAsync.queryByQueryFallback` (Bool) obok `lazyFlushModulo`. Brak klucza → domyślne beeorm
(stare zachowanie). Zła wartość → wpis w `ormasync-cron.log`, cron działa dalej.
Do zbudowania potrzebny beeorm z tego brancha (setter nie jest w żadnym tagu).

## Uwagi / nie zrobione

- Deadlock w transakcji grupy nie trafia już do resolvera (springowy `FileLazyFlushDeadlockLog`),
  tylko do `log.Printf`; resolver zobaczy deadlock dopiero po wyczerpaniu retry pojedynczego query.
- Ścieżka **pojedynczego query w grupie** i **inserty**: bez zmian (autocommit, bez retry na deadlock).
- Bug w `db.go`: `_, isTimeout := ctx.Deadline()` jest zawsze true → przy `queryTimeLimit > 0` każdy
  błąd staje się 1969, więc deadlock nie zostanie rozpoznany.
- Resolver w starej ścieżce jest wołany po rollbacku — `nil` ACK-uje całą grupę (to właśnie naprawia flaga).
- Docelowo, gdyby wrócić do beeorm: włączyć na fishing dev, potem per gra; gdy się sprawdzi — usunąć starą ścieżkę.
