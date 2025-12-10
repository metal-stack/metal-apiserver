package test

import (
	"log/slog"
	"testing"

	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"google.golang.org/grpc"
)

// go test -benchmem -count 5 -run=^$ -tags integration,client -bench ^BenchmarkMasterdataStartup$ github.com/metal-stack/metal-apiserver/pkg/test
// goos: linux
// goarch: amd64
// pkg: github.com/metal-stack/metal-apiserver/pkg/test
// cpu: 12th Gen Intel(R) Core(TM) i7-1260P
// BenchmarkMasterdataStartup/Postgres-16                 1        1675796555 ns/op         3396520 B/op      24476 allocs/op
// BenchmarkMasterdataStartup/Postgres-16                 1        1301627116 ns/op         1912800 B/op      14157 allocs/op
// BenchmarkMasterdataStartup/Postgres-16                 1        1322102476 ns/op         1861624 B/op      14079 allocs/op
// BenchmarkMasterdataStartup/Postgres-16                 1        1332727301 ns/op         1872560 B/op      14110 allocs/op
// BenchmarkMasterdataStartup/Postgres-16                 1        1291609617 ns/op         1894272 B/op      14075 allocs/op
// BenchmarkMasterdataStartup/Cockroach-16                1        2059231746 ns/op        10188480 B/op     139429 allocs/op
// BenchmarkMasterdataStartup/Cockroach-16                1        1982464710 ns/op         5829688 B/op      98106 allocs/op
// BenchmarkMasterdataStartup/Cockroach-16                1        2024728577 ns/op         5847784 B/op      98109 allocs/op
// BenchmarkMasterdataStartup/Cockroach-16                1        1938621257 ns/op         5869448 B/op      98136 allocs/op
// BenchmarkMasterdataStartup/Cockroach-16                1        2040372359 ns/op         5829944 B/op      98115 allocs/op
// PASS
// ok      github.com/metal-stack/metal-apiserver/pkg/test 18.410s

func BenchmarkMasterdataStartup(b *testing.B) {
	log := slog.New(slog.DiscardHandler)

	benchmarks := []struct {
		name        string
		dbstartfunc func(t testing.TB, log *slog.Logger) (mdc.Client, *grpc.ClientConn, func())
	}{
		{
			name:        "Postgres",
			dbstartfunc: StartMasterdataWithPostgres,
		},
		{
			name:        "Cockroach",
			dbstartfunc: StartMasterdataWithCockroach,
		},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				_, _, closer = bm.dbstartfunc(b, log)
				defer closer()
			}
		})
	}
}
