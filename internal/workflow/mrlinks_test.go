package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/penwyp/catmit/internal/git"
	"github.com/stretchr/testify/assert"
)

// fakeMRCommitter embeds the git.Committer interface and only implements the
// MR link reporting used by printMRLinks.
type fakeMRCommitter struct {
	git.Committer
	links []git.MRLink
}

func (f *fakeMRCommitter) LastPushMRLinks() []git.MRLink {
	return f.links
}

func TestPrintMRLinksWritesGongfengLinks(t *testing.T) {
	links := []git.MRLink{
		{Label: "Open in TAPD", URL: "https://tapd.woa.com/tapd_fe/dev/gongfeng/merge_requests/create"},
		{Label: "Open in TGIT", URL: "https://git.woa.com/nosql/oss_backend/-/merge_requests/new"},
	}

	var out bytes.Buffer
	printMRLinks(&out, &fakeMRCommitter{links: links})

	assert.Contains(t, out.String(), "Merge request links:")
	assert.Contains(t, out.String(), "Open in TAPD: https://tapd.woa.com/")
	assert.Contains(t, out.String(), "Open in TGIT: https://git.woa.com/nosql/oss_backend/-/merge_requests/new")
}

func TestPrintMRLinksWritesNothingWithoutLinks(t *testing.T) {
	var out bytes.Buffer
	printMRLinks(&out, &fakeMRCommitter{})

	assert.Empty(t, out.String())
}

func TestPrintMRLinksIgnoresUnsupportedCommitters(t *testing.T) {
	var out bytes.Buffer

	// A committer that does not report MR links must be ignored silently.
	printMRLinks(&out, nil)
	assert.Empty(t, out.String())

	printMRLinks(&out, context.Background())
	assert.Empty(t, out.String())
}
