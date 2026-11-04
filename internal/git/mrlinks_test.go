package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// gongfengPushOutput is the console output of a real push to a git.woa.com
// repository, whose pre-receive hook prints the TAPD and TGIT MR shortcuts.
const gongfengPushOutput = `Enumerating objects: 54, done.
Counting objects: 100% (54/54), done.
Delta compression using up to 14 threads
Compressing objects: 100% (28/28), done.
Writing objects: 100% (29/29), 30.78 KiB | 10.26 MiB/s, done.
Total 29 (delta 21), reused 0 (delta 0), pack-reused 0 (from 0)
remote: Resolving deltas: 100% (21/21)
remote: Processing pre-receive: push-check: 2, done
remote:
remote: +---------------------------------------------------------------------------------------------------------------------------------------------------------------------+
remote: | To create a merge request for feat/penwu-lint:                                                                                                                      |
remote: |     Open in TAPD: https://tapd.woa.com/tapd_fe/dev/gongfeng/merge_requests/create?from=develop&sourceBranch=feat%2Fpenwu-lint&sourceProjectPath=nosql%2Foss_backend |
remote: |     Open in TGIT: https://git.woa.com/nosql/oss_backend/-/merge_requests/new?ADTAG=git-cli&merge_request%5Bsource_branch%5D=feat%2Fpenwu-lint                       |
remote: +---------------------------------------------------------------------------------------------------------------------------------------------------------------------+
remote:
To git.woa.com:nosql/oss_backend.git
 + ddf2266b...2f6a2b9f feat/penwu-lint -> feat/penwu-lint (forced update)
branch 'feat/penwu-lint' set up to track 'origin/feat/penwu-lint'.
`

func TestHostFromRemoteURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		expect string
	}{
		{"scp like ssh", "git@git.woa.com:nosql/oss_backend.git", "git.woa.com"},
		{"ssh scheme", "ssh://git@git.woa.com:22/nosql/oss_backend.git", "git.woa.com"},
		{"https", "https://git.woa.com/nosql/oss_backend.git", "git.woa.com"},
		{"https with port", "https://git.woa.com:8443/nosql/x.git", "git.woa.com"},
		{"upper case", "git@Git.WOA.com:nosql/x.git", "git.woa.com"},
		{"github", "git@github.com:penwyp/catmit.git", "github.com"},
		{"empty", "   ", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expect, HostFromRemoteURL(c.url))
		})
	}
}

func TestIsGongfengRemote(t *testing.T) {
	assert.True(t, IsGongfengRemote("git@git.woa.com:nosql/oss_backend.git"))
	assert.True(t, IsGongfengRemote("https://Git.WOA.com/nosql/oss_backend.git"))
	assert.False(t, IsGongfengRemote("https://www.git.woa.com/nosql/oss_backend.git"))
	assert.False(t, IsGongfengRemote("https://other.git.woa.com/nosql/oss_backend.git"))
	assert.False(t, IsGongfengRemote("git@github.com:penwyp/catmit.git"))
	assert.False(t, IsGongfengRemote("git@notgit.woa.com:x/y.git"))
	assert.False(t, IsGongfengRemote(""))
}

func TestHostFromPushOutput(t *testing.T) {
	assert.Equal(t, "git.woa.com", HostFromPushOutput(gongfengPushOutput))
	assert.Equal(t, "", HostFromPushOutput("nothing to see"))
}

func TestParseMRLinks(t *testing.T) {
	links := ParseMRLinks(gongfengPushOutput)
	assert.Len(t, links, 2)

	assert.Equal(t, "Open in TAPD", links[0].Label)
	assert.Equal(t, "https://tapd.woa.com/tapd_fe/dev/gongfeng/merge_requests/create?from=develop&sourceBranch=feat%2Fpenwu-lint&sourceProjectPath=nosql%2Foss_backend", links[0].URL)

	assert.Equal(t, "Open in TGIT", links[1].Label)
	assert.Equal(t, "https://git.woa.com/nosql/oss_backend/-/merge_requests/new?ADTAG=git-cli&merge_request%5Bsource_branch%5D=feat%2Fpenwu-lint", links[1].URL)
}

func TestParseMRLinksIgnoresDuplicatesAndNoise(t *testing.T) {
	output := "remote: | see https://git.woa.com/a/b/-/merge_requests/new?x=1 |\n" +
		"remote: | again https://git.woa.com/a/b/-/merge_requests/new?x=1\n" +
		"remote: | see https://git.woa.com/a/b/-/merge_requests/new?x=1.\n"

	links := ParseMRLinks(output)
	assert.Len(t, links, 1)
	assert.Equal(t, "https://git.woa.com/a/b/-/merge_requests/new?x=1", links[0].URL)
	assert.Equal(t, "see", links[0].Label)
}

func TestParseMRLinksStripsANSIEscapes(t *testing.T) {
	output := "\x1b[33mremote: | Open in TGIT: https://git.woa.com/a/b/-/merge_requests/new?x=1\x1b[0m\n"
	links := ParseMRLinks(output)
	assert.Len(t, links, 1)
	assert.Equal(t, "https://git.woa.com/a/b/-/merge_requests/new?x=1", links[0].URL)
	assert.Equal(t, "Open in TGIT", links[0].Label)
}

func TestParseMRLinksFallsBackToDefaultLabel(t *testing.T) {
	links := ParseMRLinks("remote: https://tapd.woa.com/tapd_fe/x\n")
	assert.Len(t, links, 1)
	assert.Equal(t, "TAPD", links[0].Label)
	assert.Equal(t, "https://tapd.woa.com/tapd_fe/x", links[0].URL)
}

func TestParseMRLinksEmptyOutput(t *testing.T) {
	assert.Nil(t, ParseMRLinks(""))
	assert.Nil(t, ParseMRLinks("   \n"))
}

func TestMRLinksFromPush(t *testing.T) {
	// git.woa.com remote: links are extracted.
	links := MRLinksFromPush("git@git.woa.com:nosql/oss_backend.git", gongfengPushOutput)
	assert.Len(t, links, 2)

	// Any other remote: no links, even if the output contains URLs.
	assert.Empty(t, MRLinksFromPush("git@github.com:penwyp/catmit.git", gongfengPushOutput))
	assert.Empty(t, MRLinksFromPush("git@other.git.woa.com:nosql/oss_backend.git", gongfengPushOutput))

	// Unknown remote URL: fall back to the destination reported by git.
	assert.Len(t, MRLinksFromPush("", gongfengPushOutput), 2)
	assert.Empty(t, MRLinksFromPush("", strings.ReplaceAll(gongfengPushOutput, "git.woa.com", "other.git.woa.com")))
}

func TestFormatMRLinks(t *testing.T) {
	links := ParseMRLinks(gongfengPushOutput)
	block := FormatMRLinks(links)

	assert.Contains(t, block, "Merge request links:")
	assert.Contains(t, block, "Open in TAPD: https://tapd.woa.com/")
	assert.Contains(t, block, "Open in TGIT: https://git.woa.com/nosql/oss_backend/-/merge_requests/new")
	assert.Equal(t, 4, strings.Count(block, "\n"))

	assert.Equal(t, "", FormatMRLinks(nil))
}
