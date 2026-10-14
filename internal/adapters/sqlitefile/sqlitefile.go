// Package sqlitefile opens uploaded SQLite database files strictly read-only.
//
// The bytes are served through an immutable in-process VFS, so nothing is
// written to disk, no journal or WAL is created, and the uploaded file can never
// be modified. Each connection also enables query_only and disables
// trusted_schema so schema-defined functions in an untrusted file cannot run
// with elevated trust.
package sqlitefile

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/util/ioutil"
	"github.com/ncruces/go-sqlite3/vfs/readervfs"
)

// Header is the magic string every SQLite 3 database file starts with.
const Header = "SQLite format 3\x00"

var ErrNotSQLite = errors.New("content is not a SQLite 3 database")

// IsDatabase reports whether content starts with the SQLite 3 header.
func IsDatabase(content []byte) bool {
	return len(content) >= 512 && bytes.HasPrefix(content, []byte(Header))
}

// Open returns a read-only connection over content and a function that
// releases it. The caller must not mutate content while the connection is open.
func Open(ctx context.Context, content []byte) (*sqlite3.Conn, func(), error) {
	if !IsDatabase(content) {
		return nil, nil, ErrNotSQLite
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, nil, err
	}
	name := "semlia-" + hex.EncodeToString(random) + ".db"
	readervfs.Create(name, ioutil.NewSizeReaderAt(bytes.NewReader(content)))
	uri := "file:" + url.PathEscape(name) + "?" + url.Values{"vfs": {"reader"}, "mode": {"ro"}}.Encode()
	conn, err := sqlite3.OpenFlags(uri, sqlite3.OPEN_READONLY|sqlite3.OPEN_URI)
	if err != nil {
		readervfs.Delete(name)
		return nil, nil, ErrNotSQLite
	}
	release := func() {
		_ = conn.Close()
		readervfs.Delete(name)
	}
	conn.SetInterrupt(ctx)
	if err := conn.Exec("PRAGMA query_only = ON; PRAGMA trusted_schema = OFF;"); err != nil {
		release()
		return nil, nil, ErrNotSQLite
	}
	return conn, release, nil
}
