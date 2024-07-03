### Purpose of this module

opendb module provides observability and configurability for rocksdb databases.

### Observability

When you open RocksDB with provided API:
- opendb registers prometheus metrics
- opendb launches a `reportMetrics` goroutine which every `rocksdb.report-metrics-interval-secs` seconds reports metrics to prometheus, `reportMetrics` goroutine performs following steps:
  - uses grocksdb API to gather rocksdb properties/stats in text format
    - more info can be found here: https://github.com/facebook/rocksdb/wiki/RocksDB-Tuning-Guide#rocksdb-statistics
  - parses rocksdb properties/stats from text and reports them as prometheus metrics
 
List of reported metrics and their documentation can be found in:
- source code: `registerMetrics()` function in `metrics.go`
- corresponding grafana dashboard
 
### Configurability

opendb allows you to override default rocksdb settings, more info can be found here:
- https://github.com/facebook/rocksdb/wiki/Setup-Options-and-Basic-Tuning
- https://github.com/facebook/rocksdb/wiki/RocksDB-Tuning-Guide

There are 2 main scenarios:
- create a new rocksdb database
- open already existing database

#### Create a new rocksdb database

- opendb starts with default cometbft-db rocksdb configuration, see for the details: https://github.com/Kava-Labs/cometbft-db/blob/main/rocksdb.go
- opendb overrides options which explicitly specified in appOpts (app.toml)

#### Open an already existing database

- opendb loads stored rocksdb configuration and starts with it
- opendb overrides options which explicitly specified in appOpts (app.toml)
