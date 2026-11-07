//go:build ignore

// Generate the legacy BoltDB fixture from the pinned OLD Atmos module:
// go run /path/to/tests/compatibility/generate_bolt.go /path/to/data/legacy.db
// The current Atmos module intentionally does not depend on bbolt.
package main

import (
	"encoding/binary"
	"os"

	bolt "go.etcd.io/bbolt"
)

func main() {
	db, err := bolt.Open(os.Args[1], 0o600, &bolt.Options{PageSize: 4096})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("compat"))
		if err != nil {
			return err
		}
		for _, item := range [][2]string{{"config.json", `{"name":"bolt-fixture","count":42}`}, {"plain", "fixture-value"}} {
			value := make([]byte, 8)
			binary.LittleEndian.PutUint64(value, 1)
			if err := bucket.Put([]byte(item[0]), append(value, []byte(item[1])...)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}
