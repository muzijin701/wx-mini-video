package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MuZiJin701/wx-mini-video/internal/tui/logo"
)

func TestLogoRemainsVisibleInCompactWindows(t *testing.T) {
	cases := []struct {
		width    int
		height   int
		wantRows int
	}{
		{110, 40, 7},
		{110, 24, 5},
		{80, 24, 5},
		{60, 20, 3},
	}
	for _, tc := range cases {
		m := mediaTestModel(t)
		m.width, m.height = tc.width, tc.height
		m.logoCap = logo.Capability{Protocol: logo.ProtocolSixel, CellWidth: 10, CellHeight: 20}
		var output bytes.Buffer
		m.painter = logo.NewPainter(&output, m.logoCap)
		_, rows, ok := m.logoSize(m.metricsNow())
		if !ok || rows != tc.wantRows {
			t.Fatalf("%dx%d: logo rows=%d available=%v, want rows=%d", tc.width, tc.height, rows, ok, tc.wantRows)
		}
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != tc.height {
			t.Fatalf("%dx%d: got %d lines", tc.width, tc.height, len(lines))
		}
		assertLineWidths(t, m, lines)
		if _, err := m.painter.Write([]byte(view)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(output.Bytes(), []byte("\x1bP")) {
			t.Fatalf("%dx%d: logo not drawn", tc.width, tc.height)
		}
	}
}
