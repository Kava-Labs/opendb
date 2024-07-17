//go:build rocksdb
// +build rocksdb

// Copyright 2023 Kava Labs, Inc.
// Copyright 2023 Cronos Labs, Inc.
//
// Derived from https://github.com/crypto-org-chain/cronos@496ce7e
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opendb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/linxGnu/grocksdb"
	"github.com/spf13/cast"
)

var ErrUnexpectedConfiguration = errors.New("unexpected rocksdb configuration, rocksdb should have only one column family named default")

const (
	// default tm-db block cache size for RocksDB
	defaultBlockCacheSize = 1 << 30

	DefaultColumnFamilyName = "default"

	enableMetricsOptName             = "enable-metrics"
	reportMetricsIntervalSecsOptName = "report-metrics-interval-secs"
	defaultReportMetricsIntervalSecs = 15

	maxOpenFilesDBOptName           = "max-open-files"
	maxFileOpeningThreadsDBOptName  = "max-file-opening-threads"
	tableCacheNumshardbitsDBOptName = "table_cache_numshardbits"
	allowMMAPWritesDBOptName        = "allow_mmap_writes"
	allowMMAPReadsDBOptName         = "allow_mmap_reads"
	useFsyncDBOptName               = "use_fsync"
	useAdaptiveMutexDBOptName       = "use_adaptive_mutex"
	bytesPerSyncDBOptName           = "bytes_per_sync"
	maxBackgroundJobsDBOptName      = "max-background-jobs"

	writeBufferSizeCFOptName                = "write-buffer-size"
	numLevelsCFOptName                      = "num-levels"
	maxWriteBufferNumberCFOptName           = "max_write_buffer_number"
	minWriteBufferNumberToMergeCFOptName    = "min_write_buffer_number_to_merge"
	maxBytesForLevelBaseCFOptName           = "max_bytes_for_level_base"
	maxBytesForLevelMultiplierCFOptName     = "max_bytes_for_level_multiplier"
	targetFileSizeBaseCFOptName             = "target_file_size_base"
	targetFileSizeMultiplierCFOptName       = "target_file_size_multiplier"
	level0FileNumCompactionTriggerCFOptName = "level0_file_num_compaction_trigger"
	level0SlowdownWritesTriggerCFOptName    = "level0_slowdown_writes_trigger"

	blockCacheSizeBBTOOptName                   = "block_cache_size"
	bitsPerKeyBBTOOptName                       = "bits_per_key"
	blockSizeBBTOOptName                        = "block_size"
	cacheIndexAndFilterBlocksBBTOOptName        = "cache_index_and_filter_blocks"
	pinL0FilterAndIndexBlocksInCacheBBTOOptName = "pin_l0_filter_and_index_blocks_in_cache"
	formatVersionBBTOOptName                    = "format_version"

	asyncIOReadOptName = "read-async-io"
)

// AppOptions is the same interface as provided by cosmos-sdk, see for details:
// https://github.com/cosmos/cosmos-sdk/blob/10465a6aabdfc9119ff187ac3ef229f33c06ab45/server/types/app.go#L29-L31
// We added it here to avoid cosmos-sdk dependency.
type AppOptions interface {
	Get(string) interface{}
}

// rocksDBOptions implements AppOptions interface.
// It does it by wrapping another AppOptions, but also takes into account dbName.
type rocksDBOptions struct {
	appOpts AppOptions
	dbName  string
}

func newRocksDBOptions(appOpts AppOptions, dbName string) *rocksDBOptions {
	return &rocksDBOptions{
		appOpts: appOpts,
		dbName:  dbName,
	}
}

// Get constructs database-specific and fallback keys and use them to get value from underlying AppOptions.
// Database-specific key takes precedence over fallback key.
func (opts *rocksDBOptions) Get(key string) interface{} {
	// get value using database-specific key
	dbSpecificKey := fmt.Sprintf("rocksdb.%v.%v", opts.dbName, key)
	if opts.appOpts.Get(dbSpecificKey) != nil {
		return opts.appOpts.Get(dbSpecificKey)
	}

	// get value using fallback key
	fallbackKey := fmt.Sprintf("rocksdb.%v", key)
	return opts.appOpts.Get(fallbackKey)
}

func OpenDB(appOpts AppOptions, dataDir string, dbName string, backendType dbm.BackendType) (dbm.DB, error) {
	// wrap AppOptions with rocksDBOptions to make sure dbName is considered when applying configuration
	// it allows individual database configuration
	rocksDBOpts := newRocksDBOptions(appOpts, dbName)
	if backendType == dbm.RocksDBBackend {
		return openRocksdb(dataDir, dbName, rocksDBOpts)
	}

	return dbm.NewDB(dbName, backendType, dataDir)
}

// openRocksdb loads existing options, overrides some of them with appOpts and opens database
// option will be overridden only in case if it explicitly specified in appOpts
func openRocksdb(dir string, dbName string, appOpts AppOptions) (dbm.DB, error) {
	optionsPath := filepath.Join(dir, dbName+".db")
	dbOpts, cfOpts, err := LoadLatestOptions(optionsPath)
	if err != nil {
		return nil, err
	}
	// customize rocksdb options
	bbtoOpts := bbtoFromAppOpts(appOpts)
	dbOpts.SetBlockBasedTableFactory(bbtoOpts)
	cfOpts.SetBlockBasedTableFactory(bbtoOpts)
	dbOpts = overrideDBOpts(dbOpts, appOpts)
	cfOpts = overrideCFOpts(cfOpts, appOpts)
	readOpts := readOptsFromAppOpts(appOpts)

	enableMetrics := cast.ToBool(appOpts.Get(enableMetricsOptName))
	reportMetricsIntervalSecs := cast.ToInt64(appOpts.Get(reportMetricsIntervalSecsOptName))
	if reportMetricsIntervalSecs == 0 {
		reportMetricsIntervalSecs = defaultReportMetricsIntervalSecs
	}

	return newRocksDBWithOptions(dbName, dir, dbOpts, cfOpts, readOpts, enableMetrics, reportMetricsIntervalSecs)
}

// LoadLatestOptions loads and returns database and column family options
// if options file not found, it means database isn't created yet, in such case default tm-db options will be returned
// if database exists it should have only one column family named default
func LoadLatestOptions(dir string) (*grocksdb.Options, *grocksdb.Options, error) {
	latestOpts, err := grocksdb.LoadLatestOptions(dir, grocksdb.NewDefaultEnv(), true, grocksdb.NewLRUCache(defaultBlockCacheSize))
	if err != nil && strings.HasPrefix(err.Error(), "NotFound: ") {
		return newDefaultOptions(), newDefaultOptions(), nil
	}
	if err != nil {
		return nil, nil, err
	}

	cfNames := latestOpts.ColumnFamilyNames()
	cfOpts := latestOpts.ColumnFamilyOpts()
	// db should have only one column family named default
	ok := len(cfNames) == 1 && cfNames[0] == DefaultColumnFamilyName
	if !ok {
		return nil, nil, ErrUnexpectedConfiguration
	}

	// return db and cf opts
	return latestOpts.Options(), &cfOpts[0], nil
}

// overrideDBOpts merges dbOpts and appOpts, appOpts takes precedence
func overrideDBOpts(dbOpts *grocksdb.Options, appOpts AppOptions) *grocksdb.Options {
	maxOpenFiles := appOpts.Get(maxOpenFilesDBOptName)
	if maxOpenFiles != nil {
		dbOpts.SetMaxOpenFiles(cast.ToInt(maxOpenFiles))
	}

	maxFileOpeningThreads := appOpts.Get(maxFileOpeningThreadsDBOptName)
	if maxFileOpeningThreads != nil {
		dbOpts.SetMaxFileOpeningThreads(cast.ToInt(maxFileOpeningThreads))
	}

	tableCacheNumshardbits := appOpts.Get(tableCacheNumshardbitsDBOptName)
	if tableCacheNumshardbits != nil {
		dbOpts.SetTableCacheNumshardbits(cast.ToInt(tableCacheNumshardbits))
	}

	allowMMAPWrites := appOpts.Get(allowMMAPWritesDBOptName)
	if allowMMAPWrites != nil {
		dbOpts.SetAllowMmapWrites(cast.ToBool(allowMMAPWrites))
	}

	allowMMAPReads := appOpts.Get(allowMMAPReadsDBOptName)
	if allowMMAPReads != nil {
		dbOpts.SetAllowMmapReads(cast.ToBool(allowMMAPReads))
	}

	useFsync := appOpts.Get(useFsyncDBOptName)
	if useFsync != nil {
		dbOpts.SetUseFsync(cast.ToBool(useFsync))
	}

	useAdaptiveMutex := appOpts.Get(useAdaptiveMutexDBOptName)
	if useAdaptiveMutex != nil {
		dbOpts.SetUseAdaptiveMutex(cast.ToBool(useAdaptiveMutex))
	}

	bytesPerSync := appOpts.Get(bytesPerSyncDBOptName)
	if bytesPerSync != nil {
		dbOpts.SetBytesPerSync(cast.ToUint64(bytesPerSync))
	}

	maxBackgroundJobs := appOpts.Get(maxBackgroundJobsDBOptName)
	if maxBackgroundJobs != nil {
		dbOpts.SetMaxBackgroundJobs(cast.ToInt(maxBackgroundJobs))
	}

	return dbOpts
}

// overrideCFOpts merges cfOpts and appOpts, appOpts takes precedence
func overrideCFOpts(cfOpts *grocksdb.Options, appOpts AppOptions) *grocksdb.Options {
	writeBufferSize := appOpts.Get(writeBufferSizeCFOptName)
	if writeBufferSize != nil {
		cfOpts.SetWriteBufferSize(cast.ToUint64(writeBufferSize))
	}

	numLevels := appOpts.Get(numLevelsCFOptName)
	if numLevels != nil {
		cfOpts.SetNumLevels(cast.ToInt(numLevels))
	}

	maxWriteBufferNumber := appOpts.Get(maxWriteBufferNumberCFOptName)
	if maxWriteBufferNumber != nil {
		cfOpts.SetMaxWriteBufferNumber(cast.ToInt(maxWriteBufferNumber))
	}

	minWriteBufferNumberToMerge := appOpts.Get(minWriteBufferNumberToMergeCFOptName)
	if minWriteBufferNumberToMerge != nil {
		cfOpts.SetMinWriteBufferNumberToMerge(cast.ToInt(minWriteBufferNumberToMerge))
	}

	maxBytesForLevelBase := appOpts.Get(maxBytesForLevelBaseCFOptName)
	if maxBytesForLevelBase != nil {
		cfOpts.SetMaxBytesForLevelBase(cast.ToUint64(maxBytesForLevelBase))
	}

	maxBytesForLevelMultiplier := appOpts.Get(maxBytesForLevelMultiplierCFOptName)
	if maxBytesForLevelMultiplier != nil {
		cfOpts.SetMaxBytesForLevelMultiplier(cast.ToFloat64(maxBytesForLevelMultiplier))
	}

	targetFileSizeBase := appOpts.Get(targetFileSizeBaseCFOptName)
	if targetFileSizeBase != nil {
		cfOpts.SetTargetFileSizeBase(cast.ToUint64(targetFileSizeBase))
	}

	targetFileSizeMultiplier := appOpts.Get(targetFileSizeMultiplierCFOptName)
	if targetFileSizeMultiplier != nil {
		cfOpts.SetTargetFileSizeMultiplier(cast.ToInt(targetFileSizeMultiplier))
	}

	level0FileNumCompactionTrigger := appOpts.Get(level0FileNumCompactionTriggerCFOptName)
	if level0FileNumCompactionTrigger != nil {
		cfOpts.SetLevel0FileNumCompactionTrigger(cast.ToInt(level0FileNumCompactionTrigger))
	}

	level0SlowdownWritesTrigger := appOpts.Get(level0SlowdownWritesTriggerCFOptName)
	if level0SlowdownWritesTrigger != nil {
		cfOpts.SetLevel0SlowdownWritesTrigger(cast.ToInt(level0SlowdownWritesTrigger))
	}

	return cfOpts
}

func readOptsFromAppOpts(appOpts AppOptions) *grocksdb.ReadOptions {
	ro := grocksdb.NewDefaultReadOptions()
	asyncIO := appOpts.Get(asyncIOReadOptName)
	if asyncIO != nil {
		ro.SetAsyncIO(cast.ToBool(asyncIO))
	}

	return ro
}

func bbtoFromAppOpts(appOpts AppOptions) *grocksdb.BlockBasedTableOptions {
	bbto := defaultBBTO()

	blockCacheSize := appOpts.Get(blockCacheSizeBBTOOptName)
	if blockCacheSize != nil {
		cache := grocksdb.NewLRUCache(cast.ToUint64(blockCacheSize))
		bbto.SetBlockCache(cache)
	}

	bitsPerKey := appOpts.Get(bitsPerKeyBBTOOptName)
	if bitsPerKey != nil {
		filter := grocksdb.NewBloomFilter(cast.ToFloat64(bitsPerKey))
		bbto.SetFilterPolicy(filter)
	}

	blockSize := appOpts.Get(blockSizeBBTOOptName)
	if blockSize != nil {
		bbto.SetBlockSize(cast.ToInt(blockSize))
	}

	cacheIndexAndFilterBlocks := appOpts.Get(cacheIndexAndFilterBlocksBBTOOptName)
	if cacheIndexAndFilterBlocks != nil {
		bbto.SetCacheIndexAndFilterBlocks(cast.ToBool(cacheIndexAndFilterBlocks))
	}

	pinL0FilterAndIndexBlocksInCache := appOpts.Get(pinL0FilterAndIndexBlocksInCacheBBTOOptName)
	if pinL0FilterAndIndexBlocksInCache != nil {
		bbto.SetPinL0FilterAndIndexBlocksInCache(cast.ToBool(pinL0FilterAndIndexBlocksInCache))
	}

	formatVersion := appOpts.Get(formatVersionBBTOOptName)
	if formatVersion != nil {
		bbto.SetFormatVersion(cast.ToInt(formatVersion))
	}

	return bbto
}

// newRocksDBWithOptions opens rocksdb with provided database and column family options
// newRocksDBWithOptions expects that db has only one column family named default
func newRocksDBWithOptions(
	dbName string,
	dir string,
	dbOpts *grocksdb.Options,
	cfOpts *grocksdb.Options,
	readOpts *grocksdb.ReadOptions,
	enableMetrics bool,
	reportMetricsIntervalSecs int64,
) (*dbm.RocksDB, error) {
	dbPath := filepath.Join(dir, dbName+".db")

	// Ensure path exists
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db path: %w", err)
	}

	// EnableStatistics adds overhead so shouldn't be enabled in production
	if enableMetrics {
		dbOpts.EnableStatistics()
	}

	db, _, err := grocksdb.OpenDbColumnFamilies(dbOpts, dbPath, []string{DefaultColumnFamilyName}, []*grocksdb.Options{cfOpts})
	if err != nil {
		return nil, err
	}

	if enableMetrics {
		registerMetrics()
		go reportMetrics(dbName, db, time.Second*time.Duration(reportMetricsIntervalSecs))
	}

	wo := grocksdb.NewDefaultWriteOptions()
	woSync := grocksdb.NewDefaultWriteOptions()
	woSync.SetSync(true)
	return dbm.NewRocksDBWithRawDB(db, readOpts, wo, woSync), nil
}

// newDefaultOptions returns default tm-db options for RocksDB, see for details:
// https://github.com/Kava-Labs/tm-db/blob/94ff76d31724965f8883cddebabe91e0d01bc03f/rocksdb.go#L30
func newDefaultOptions() *grocksdb.Options {
	// default rocksdb option, good enough for most cases, including heavy workloads.
	// 1GB table cache, 512MB write buffer(may use 50% more on heavy workloads).
	// compression: snappy as default, need to -lsnappy to enable.
	bbto := defaultBBTO()

	opts := grocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	// SetMaxOpenFiles to 4096 seems to provide a reliable performance boost
	opts.SetMaxOpenFiles(4096)
	opts.SetCreateIfMissing(true)
	opts.IncreaseParallelism(runtime.NumCPU())
	// 1.5GB maximum memory use for writebuffer.
	opts.OptimizeLevelStyleCompaction(512 * 1024 * 1024)

	return opts
}

// defaultBBTO returns default tm-db bbto options for RocksDB, see for details:
// https://github.com/Kava-Labs/tm-db/blob/94ff76d31724965f8883cddebabe91e0d01bc03f/rocksdb.go#L30
func defaultBBTO() *grocksdb.BlockBasedTableOptions {
	bbto := grocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockCache(grocksdb.NewLRUCache(defaultBlockCacheSize))
	bbto.SetFilterPolicy(grocksdb.NewBloomFilter(10))

	return bbto
}

// reportMetrics periodically requests stats from rocksdb and reports to prometheus
// NOTE: should be launched as a goroutine
func reportMetrics(dbName string, db *grocksdb.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			props, stats, err := getPropsAndStats(db)
			if err != nil {
				continue
			}

			if rocksdbMetrics == nil {
				continue
			}
			rocksdbMetrics.report(dbName, props, stats)
		}
	}
}

// getPropsAndStats gets statistics from rocksdb
func getPropsAndStats(db *grocksdb.DB) (*properties, *stats, error) {
	propsLoader := newPropsLoader(db)
	props, err := propsLoader.load()
	if err != nil {
		return nil, nil, err
	}

	statMap, err := parseSerializedStats(props.OptionsStatistics)
	if err != nil {
		return nil, nil, err
	}

	statLoader := newStatLoader(statMap)
	stats, err := statLoader.load()
	if err != nil {
		return nil, nil, err
	}

	return props, stats, nil
}
