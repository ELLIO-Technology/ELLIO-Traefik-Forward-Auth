package auth

// Version information - set at build time via ldflags
var (
	version   string
	gitCommit string
	buildDate string
)

// SetVersionInfo sets the version information from main package
func SetVersionInfo(v, gc, bd string) {
	version = v
	gitCommit = gc
	buildDate = bd
}

func getVersion() string {
	if version == "" {
		return "dev"
	}
	return version
}

func getGitCommit() string {
	if gitCommit == "" {
		return "unknown"
	}
	return gitCommit
}

func getBuildDate() string {
	if buildDate == "" {
		return "unknown"
	}
	return buildDate
}