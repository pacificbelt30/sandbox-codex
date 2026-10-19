package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/proxysettings"
	"github.com/rivo/tview"
)

// RunProxySettings also works when the node is unavailable.
func RunProxySettings(path string) error {
	app := tview.NewApplication()
	return app.SetRoot(proxySettingsPage(path, app.Stop), true).EnableMouse(true).Run()
}

func proxySettingsPage(path string, back func()) tview.Primitive {
	status := tview.NewTextView().SetDynamicColors(false)
	status.SetText("Changes apply when creating new proxies/workers. Running policies stay immutable.")
	fields := make([]*tview.InputField, 8)
	form := tview.NewForm()
	labels := []string{"Allow domains", "Deny domains", "Allow CIDRs", "Deny CIDRs", "Allow ports", "Deny ports", "Upstream proxy URL", "Host CA bundle path"}
	for i, label := range labels {
		fields[i] = tview.NewInputField().SetLabel(label + ": ").SetFieldWidth(60)
		form.AddFormItem(fields[i])
	}
	destination := tview.NewInputField().SetLabel("Test HOST:PORT: ").SetFieldWidth(60)
	form.AddFormItem(destination)
	var rev string
	load := func() {
		s, revision, err := proxysettings.Load(path)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		rev = revision
		values := []string{strings.Join(s.AllowDomains, ", "), strings.Join(s.DenyDomains, ", "), strings.Join(s.AllowCIDRs, ", "), strings.Join(s.DenyCIDRs, ", "), portsText(s.AllowPorts), portsText(s.DenyPorts), s.UpstreamProxy, s.CAFile}
		for i, value := range values {
			fields[i].SetText(value)
		}
		status.SetText("Loaded " + path + "\nChanges apply to new proxies/workers; existing policies remain unchanged.")
	}
	collect := func() (proxysettings.Settings, error) {
		s := proxysettings.Settings{UpstreamProxy: strings.TrimSpace(fields[6].GetText()), CAFile: strings.TrimSpace(fields[7].GetText()), AllowDomains: proxysettings.Split(fields[0].GetText()), DenyDomains: proxysettings.Split(fields[1].GetText()), AllowCIDRs: proxysettings.Split(fields[2].GetText()), DenyCIDRs: proxysettings.Split(fields[3].GetText())}
		var err error
		s.AllowPorts, err = proxysettings.ParsePorts(fields[4].GetText())
		if err != nil {
			return s, err
		}
		s.DenyPorts, err = proxysettings.ParsePorts(fields[5].GetText())
		if err != nil {
			return s, err
		}
		return s, s.Validate()
	}
	form.AddButton("Validate", func() {
		s, err := collect()
		if err != nil {
			status.SetText(err.Error())
			return
		}
		status.SetText("Valid. Policy " + s.PolicyID())
	})
	form.AddButton("Test destination", func() {
		s, err := collect()
		if err != nil {
			status.SetText(err.Error())
			return
		}
		d, err := networkpolicy.Explain(context.Background(), s.Rules(), destination.GetText(), false, nil)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		status.SetText(fmt.Sprintf("allowed=%t reason=%s; name/port only. DNS and connections were not tested.", d.Allowed, d.Code))
	})
	form.AddButton("Save", func() {
		s, err := collect()
		if err == nil {
			err = proxysettings.Save(path, rev, s)
		}
		if err != nil {
			status.SetText(err.Error())
			return
		}
		load()
		status.SetText("Saved. Recreate the desired proxies/workers explicitly to apply the new settings.")
	})
	form.AddButton("Reload", load).AddButton("Back", back)
	form.SetBorder(true).SetTitle(" Proxy policy settings ")
	form.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			back()
			return nil
		}
		return e
	})
	load()
	return tview.NewFlex().SetDirection(tview.FlexRow).AddItem(form, 0, 1, true).AddItem(status, 4, 0, false)
}
func portsText(ports []int) string {
	var text []string
	for _, p := range ports {
		text = append(text, strconv.Itoa(p))
	}
	return strings.Join(text, ", ")
}
