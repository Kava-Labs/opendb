//go:build !rocksdb
// +build !rocksdb

package opendb

import (
	dbm "github.com/cometbft/cometbft-db"
	"github.com/cosmos/cosmos-sdk/server/types"
)

// OpenDB is a copy of default DBOpener function used by ethermint, see for details:
// https://github.com/evmos/ethermint/blob/07cf2bd2b1ce9bdb2e44ec42a39e7239292a14af/server/start.go#L647
func OpenDB(appOpts types.AppOptions, dataDir string, dbName string, backendType dbm.BackendType) (dbm.DB, error) {
	return dbm.NewDB(dbName, backendType, dataDir)
}
