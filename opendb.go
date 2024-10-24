//go:build !rocksdb
// +build !rocksdb

package opendb

import (
	dbm "github.com/cosmos/cosmos-db"
)

// AppOptions is the same interface as provided by cosmos-sdk, see for details:
// https://github.com/cosmos/cosmos-sdk/blob/10465a6aabdfc9119ff187ac3ef229f33c06ab45/server/types/app.go#L29-L31
// We added it here to avoid cosmos-sdk dependency.
type AppOptions interface {
	Get(string) interface{}
}

// OpenDB is a copy of default DBOpener function used by ethermint, see for details:
// https://github.com/evmos/ethermint/blob/07cf2bd2b1ce9bdb2e44ec42a39e7239292a14af/server/start.go#L647
func OpenDB(appOpts AppOptions, dataDir string, dbName string, backendType dbm.BackendType) (dbm.DB, error) {
	return dbm.NewDB(dbName, backendType, dataDir)
}
