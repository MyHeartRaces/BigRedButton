package buildinfo

import "strings"

var (
	Version = "0.2.1"
	Commit  = ""
	Date    = ""
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

func Current() Info {
	return Info{
		Version: strings.TrimSpace(Version),
		Commit:  strings.TrimSpace(Commit),
		Date:    strings.TrimSpace(Date),
	}
}

func DisplayVersion() string {
	info := Current()
	if info.Version == "" {
		return "dev"
	}
	return info.Version
}
