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
