package releaseinfo

import (
	"strings"
	"testing"
)

func TestReleaseMetadataRequiresStableCanonicalTag(t *testing.T) {
	for _, input := range []string{
		`{"tag_name":"v0.0.2","html_url":"https://github.com/pacificbelt30/sandbox-oasis/releases/tag/v0.0.2"}`,
	} {
		if _, err := decode(strings.NewReader(input)); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{
		`{"tag_name":"v0.0.2;command","html_url":"https://github.com/pacificbelt30/sandbox-oasis/releases/tag/v0.0.2;command"}`,
		`{"tag_name":"v0.0.2","draft":true,"html_url":"https://github.com/pacificbelt30/sandbox-oasis/releases/tag/v0.0.2"}`,
		`{"tag_name":"v0.0.2","prerelease":true,"html_url":"https://github.com/pacificbelt30/sandbox-oasis/releases/tag/v0.0.2"}`,
		`{"tag_name":"v0.0.2","html_url":"https://unrelated.example/release"}`,
		`{"tag_name":"../../unsafe"}`,
	} {
		if _, err := decode(strings.NewReader(input)); err == nil {
			t.Fatal("accepted unsafe release metadata")
		}
	}
}
