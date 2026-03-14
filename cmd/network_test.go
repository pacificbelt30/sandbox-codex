package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfirmCreateProxyNetworkYes(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("y\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateProxyNetwork(command)
	if err != nil {
		t.Fatalf("confirmCreateProxyNetwork() error = %v", err)
	}
	if !ok {
		t.Fatalf("confirmCreateProxyNetwork() = %v, want true", ok)
	}
}

func TestConfirmCreateProxyNetworkNo(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("n\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateProxyNetwork(command)
	if err != nil {
		t.Fatalf("confirmCreateProxyNetwork() error = %v", err)
	}
	if ok {
		t.Fatalf("confirmCreateProxyNetwork() = %v, want false", ok)
	}
}

func TestConfirmCreateNetworkYes(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("yes\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateNetwork(command, "dock-net")
	if err != nil {
		t.Fatalf("confirmCreateNetwork() error = %v", err)
	}
	if !ok {
		t.Fatalf("confirmCreateNetwork() = %v, want true", ok)
	}
}

func TestConfirmCreateNetworkDefaultNo(t *testing.T) {
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("\n"))
	command.SetOut(&bytes.Buffer{})

	ok, err := confirmCreateNetwork(command, "dock-net")
	if err != nil {
		t.Fatalf("confirmCreateNetwork() error = %v", err)
	}
	if ok {
		t.Fatalf("confirmCreateNetwork() = %v, want false", ok)
	}
}
