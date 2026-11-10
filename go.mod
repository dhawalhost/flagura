module github.com/dhawalhost/flagura

go 1.26.7

toolchain go1.27.3

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/a-h/templ v0.3.1100
	github.com/dhawalhost/flagura/sdks/go v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.12.3
	github.com/open-feature/go-sdk v1.20.0
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/sdk v1.47.0
	go.opentelemetry.io/otel/trace v1.47.0
	golang.org/x/crypto v0.58.1-0.20261009181718-2f1f834330b2
	golang.org/x/time v0.16.0
	modernc.org/sqlite v1.62.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.uber.org/mock v0.6.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/dhawalhost/flagura/sdks/go => ./sdks/go
