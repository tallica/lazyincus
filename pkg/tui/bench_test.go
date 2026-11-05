package tui

import (
	"fmt"
	"testing"
)

// go test -run '^$' -bench . -benchmem ./pkg/tui
var benchSizes = []struct{ width, height int }{{140, 40}, {200, 60}}

func benchEachSize(b *testing.B, op func(m *Model)) {
	b.Helper()

	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(b *testing.B) {
			m := start(b, options{fixture: true}, size.width, size.height)
			b.ReportAllocs()
			for b.Loop() {
				op(m)
			}
		})
	}
}

func BenchmarkView(b *testing.B) {
	benchEachSize(b, func(m *Model) { m.View() })
}

func BenchmarkUpdateKey(b *testing.B) {
	down := false
	benchEachSize(b, func(m *Model) {
		down = !down
		if down {
			press(m, "j")
		} else {
			press(m, "k")
		}
		m.View()
	})
}

func BenchmarkUpdateSpinner(b *testing.B) {
	benchEachSize(b, func(m *Model) { m.Update(spinnerMsg{}) })
}

func BenchmarkRefreshAll(b *testing.B) {
	benchEachSize(b, func(m *Model) { run(m, m.refreshAll()) })
}
