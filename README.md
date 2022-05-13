# padde
Passive Aggressive Dns Done Easy - PADDE

Quick and dirty passive dns

Løsningen består foreløpig av tre komponenter.

* Clickhouse installert lokalt https://clickhouse.com
* Suricata satt opp til å logge dns hendelser https://suricata.io
* Taylor (i dette repo), en enkel Go deamon som leser JSON log fra surricata og dytter inn i lokal clickhouse base

Dette er en POC, men fungererer og er testet på en probe som har peak opp til 30Gb/s

TODO:
* Legg inn kommandolinje støtte til Taylor for å angi alle parametre til clickhousebase
* Lage "noe" som kan spørre basen, enten ett API i taylor eller en egen daemon
* systemd unit fil for å starte taylor

## Installasjon

1. Installer clickhouse lokalt på surricata proben
2. Lag databasen

```
$ echo "CREATE DATABASE PADDE" | clickhouse-cli
$ clickhouse-cli < padde_log.sql
```
3. Kompiler taylor
```
$ CGO_ENABLED=0 go build tayolor.go
```
4. Start taylor med rett parametre
```
$ taylor -filename /var/log/surricata/eve-dns.json -skip TXT,DNSKEY
```

5. Les data fra basen
```
echo "SELECT * frorm padde.log" | clickhouse-cli
```

```
SELECT *
FROM padde.log
WHERE query LIKE 'github.com'
LIMIT 1

Query id: 4e669149-6531-4c2c-8b72-9dec7acb820e

┌─query──────┬─answer───────┬─qtype─┬──────first─┬───────last─┬─count─┐
│ github.com │ 140.82.112.3 │ A     │ 1649436366 │ 1649926564 │    18 │
└────────────┴──────────────┴───────┴────────────┴────────────┴───────┘
```
