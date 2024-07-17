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

### List of databases:

| Name                            | Subsystem          | IAVL V1 size as of 10.5 millions blocks | IAVL V1 number of SST files as of 10.5 millions blocks |
| ------------------------------- | ------------------ | --------------------------------------- | ------------------------------------------------------ |
| application.db                  | Kava               | 4.5 TB                                  | 76k   |
| metadata.db (snapshots)         | Kava               | 21 MB                                   | 300   |
| blockstore.db                   | Ethermint          | 337 GB                                  | 6k    |
| state.db                        | Ethermint          | 282 GB                                  | 5.5k  |
| tx_index.db                     | Ethermint          | 504 GB                                  | 11.6k |
| evidence.db                     | Ethermint          | 28 MB                                   | 270   |
| evm_indexer.db                  | Ethermint          | 2.2 GB                                  | 230   |

### List of reported rocksdb metrics:

| Name                            | Subsystem          | Docs |
| ------------------------------- | ------------------ | ---- |
| number_keys_written             | Key                |      |
| number_keys_read                | Key                |      |
| number_keys_updated             | Key                |      |
| estimate_num_keys               | Key                | estimated number of total keys in the active and unflushed immutable memtables and storage |
| number_file_opens               | File               |      |
| number_file_errors              | File               |      |
| block_cache_usage               | Memory             | memory size for the entries residing in block cache |
| estimate_table_readers_mem      | Memory             | estimated memory used for reading SST tables, excluding memory used in block cache (e.g., filter and index blocks) |
| cur_size_all_mem_tables         | Memory             | approximate size of active and unflushed immutable memtables (bytes) |
| block_cache_pinned_usage        | Memory             | returns the memory size for the entries being pinned |
| block_cache_miss                | Cache              | block_cache_miss == block_cache_index_miss + block_cache_filter_miss + block_cache_data_miss |
| block_cache_hit                 | Cache              | block_cache_hit == block_cache_index_hit + block_cache_filter_hit + block_cache_data_hit |
| block_cache_add                 | Cache              | number of blocks added to block cache |
| block_cache_add_failures        | Cache              | number of failures when adding blocks to block cache |
| block_cache_index_miss          | Detailed Cache     | |
| block_cache_index_hit           | Detailed Cache     | |
| block_cache_index_bytes_insert  | Detailed Cache     | |
| block_cache_filter_miss         | Detailed Cache     | |
| block_cache_filter_hit          | Detailed Cache     | |
| block_cache_filter_bytes_insert | Detailed Cache     | |
| block_cache_data_miss           | Detailed Cache     | |
| block_cache_data_hit            | Detailed Cache     | |
| block_cache_data_bytes_insert   | Detailed Cache     | |
| db_get_micros_p50               | Latency            | |
| db_get_micros_p95               | Latency            | |
| db_get_micros_p99               | Latency            | |
| db_get_micros_p100              | Latency            | |
| db_get_micros_count             | Latency            | |
| db_write_micros_p50             | Latency            | |
| db_write_micros_p95             | Latency            | |
| db_write_micros_p99             | Latency            | |
| db_write_micros_p100            | Latency            | |
| db_write_micros_count           | Latency            | |
| stall_micros                    | Stall              | Writer has to wait for compaction or flush to finish. |
| db_write_stall_p50              | Stall              | |
| db_write_stall_p95              | Stall              | |
| db_write_stall_p99              | Stall              | |
| db_write_stall_p100             | Stall              | |
| db_write_stall_count            | Stall              | |
| db_write_stall_sum              | Stall              | |
| bloom_filter_useful             | Filter             | number of times bloom filter has avoided file reads, i.e., negatives. |
| bloom_filter_full_positive      | Filter             | number of times bloom FullFilter has not avoided the reads. |
| bloom_filter_full_true_positive | Filter             | number of times bloom FullFilter has not avoided the reads and data actually exist. |
| last_level_read_bytes           | LSM                | |
| last_level_read_count           | LSM                | |
| non_last_level_read_bytes       | LSM                | |
| non_last_level_read_count       | LSM                | |
| get_hit_l0                      | LSM                | number of Get() queries served by L0 |
| get_hit_l1                      | LSM                | number of Get() queries served by L1 |
| get_hit_l2_and_up               | LSM                | number of Get() queries served by L2 and up |

### Example of RocksDB configuration

```yml
#####################
# RocksDB Tuning
#####################
rocksdb_enable_metrics: true
rocksdb_report_metrics_interval_secs: 15

rocksdb_max_open_files: 16384 # increase default max # of open files from 4096
rocksdb_max_file_opening_threads: 16
rocksdb_table_cache_numshardbits: 6
rocksdb_allow_mmap_writes: false
rocksdb_allow_mmap_reads: false
rocksdb_use_fsync: false
rocksdb_use_adaptive_mutex: false
rocksdb_bytes_per_sync: 0
rocksdb_max_background_jobs: 16

rocksdb_write_buffer_size: 134217728
rocksdb_num_levels: 7
rocksdb_max_write_buffer_number: 6
rocksdb_min_write_buffer_number_to_merge: 2
rocksdb_max_bytes_for_level_base: 536870912
rocksdb_max_bytes_for_level_multiplier: 10.000000
rocksdb_target_file_size_base: 67108864
rocksdb_target_file_size_multiplier: 1
rocksdb_level0_file_num_compaction_trigger: 2
rocksdb_level0_slowdown_writes_trigger: 20

rocksdb_block_cache_size: 1073741824
rocksdb_bits_per_key: 10
rocksdb_block_size: 16384 # 16K to match default zfs. Decreases block index memory usage by 4x from the default 4K
rocksdb_cache_index_and_filter_blocks: false
rocksdb_pin_l0_filter_and_index_blocks_in_cache: false
rocksdb_format_version: 5

# https://rocksdb.org/blog/2022/10/07/asynchronous-io-in-rocksdb.html
rocksdb_read_async_io: true # help speed up iterations
```
