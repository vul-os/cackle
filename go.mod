module github.com/vul-os/cackle

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.2.5
	github.com/go-chi/cors v1.2.2
	github.com/oklog/ulid/v2 v2.1.1
	golang.org/x/crypto v0.54.0
	golang.org/x/image v0.44.0
	golang.org/x/time v0.15.0
	modernc.org/sqlite v1.54.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// patala-go is ONLY imported by internal/payments/patala.go, which itself
// only compiles under `-tags patala` (see that file's build comment and
// docs/PAYMENTS.md "The patala path") — the default `go build`/`go test`
// (no tags) never reaches this import, so this requirement being present
// in go.mod does not pull cgo, or anything else, into the default build.
//
// patala-go's own Go bindings (bindings/patala/) are generated build
// output (gitignored in that repo, produced by
// `cd ../patala/patala-go && make FEATURES=fiat-all generate`), not
// something fetchable from a module proxy — hence the `replace` below
// pointing at the sibling patala checkout rather than a version+sum pair.
// A `-tags patala` build of this repo requires that sibling checkout to
// exist with its bindings already generated; see Makefile's
// `build-patala`/`test-patala` targets for the full recipe as one command.
require github.com/vul-os/patala/patala-go v0.0.0-00010101000000-000000000000

replace github.com/vul-os/patala/patala-go => ../patala/patala-go
