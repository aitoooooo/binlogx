package version

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
	GitBranch = "unknown"
)

func Info() string {
	return "binlogx version " + Version + " (" + GitBranch + "/" + GitCommit + ") built at " + BuildTime
}
