package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/pacificbelt30/codex-dock/internal/proxysettings"
	"github.com/rivo/tview"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyFormSavesOnlyPolicyAndMasksNoSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[proxy]\nadmin_secret = 'never-render-this'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	page := proxySettingsPage(path, func() {}).(*tview.Flex)
	form := page.GetItem(0).(*tview.Form)
	form.GetFormItem(0).(*tview.InputField).SetText("example.com")
	button := form.GetButton(2)
	button.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	got, _, err := proxysettings.Load(path)
	if err != nil || len(got.AllowDomains) != 1 || got.AllowDomains[0] != "example.com" {
		t.Fatalf("%+v %v", got, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "never-render-this") {
		t.Fatal("secret removed")
	}
	screen := tcell.NewSimulationScreen("")
	if err = screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	page.SetRect(0, 0, 120, 40)
	page.Draw(screen)
	var rendered strings.Builder
	for _, cell := range func() []tcell.SimCell { cells, _, _ := screen.GetContents(); return cells }() {
		rendered.Write(cell.Bytes)
	}
	if strings.Contains(rendered.String(), "never-render-this") {
		t.Fatal("secret rendered")
	}
}
