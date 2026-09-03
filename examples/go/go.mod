module github.com/dhawalhost/flagura/examples/go

go 1.25.0

require (
	github.com/dhawalhost/flagura/sdks/go v0.0.0
	github.com/open-feature/go-sdk v1.18.0
)

require go.uber.org/mock v0.6.0 // indirect

replace github.com/dhawalhost/flagura/sdks/go => ../../sdks/go
