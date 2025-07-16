package cli

// Version 语义化版本结构
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
	Build      string
}