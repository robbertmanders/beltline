package launch

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/kit/demo"
)

// scaledCopies is how many copies of each finished demo run the scaled benchmarks add: a data
// folder some months into daily use, where most runs have shipped or been dropped.
const scaledCopies = 40

// scaledDemo is a demo coordinator whose folder also holds scaledCopies copies of every finished
// demo run, each under a subject of its own, opened as a restarted daemon would open it.
func scaledDemo(b *testing.B) *coordinator.Coordinator {
	b.Helper()
	root := b.TempDir()
	setup, err := demo.Make(root, time.Now(), func(string) string { return "" })
	if err != nil {
		b.Fatal(err)
	}
	storage := store.NewStorage(store.Paths{Root: root})
	for _, run := range setup.Storage.LoadRuns() {
		if run.State.IsActive() {
			continue
		}
		for n := 1; n <= scaledCopies; n++ {
			copied := run.Clone()
			copied.Subject.Number += 1000 * n
			if copied.Subject.Key != "" {
				copied.Subject.Key += "-" + strconv.Itoa(n)
			}
			copied.ID = core.RunID(copied.BeltID, copied.Subject)
			if err := storage.SaveRun(copied); err != nil {
				b.Fatal(err)
			}
		}
	}
	c, err := LaunchSetupDemo(root, func(string, string) {}, nil)
	if err != nil {
		b.Fatal(err)
	}
	c.Tick(context.Background())
	settleTimes(b, root)
	c.Tick(context.Background())
	return c
}

// BenchmarkScaledTick times a tick over the scaled folder.
func BenchmarkScaledTick(b *testing.B) {
	c := scaledDemo(b)
	b.Logf("%d runs", len(c.Runs()))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Tick(ctx)
	}
}

// BenchmarkScaledOverview times the Overview read over the scaled folder.
func BenchmarkScaledOverview(b *testing.B) {
	c := scaledDemo(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.ReadOverviewView(nil, nil)
	}
}

// BenchmarkScaledWork times the Work read over the scaled folder.
func BenchmarkScaledWork(b *testing.B) {
	c := scaledDemo(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.ReadWorkView(core.WorkQuery{}, nil, nil)
	}
}
