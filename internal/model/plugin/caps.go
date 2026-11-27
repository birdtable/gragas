package plugin

// The caps of a step's run and read, in bytes unless the name says otherwise.
// The engine's codec judges what a plugin sends against them (core/wire), and
// host's step runner holds a run and a read to them at the read, never after
// it, so both take them from here.
const (
	// MaxStream is the most bytes of one stream of a run, which cut says was
	// cut when it was.
	MaxStream = 8 << 20
	// MaxStdin is the most bytes of a run's stdin.
	MaxStdin = 1 << 20
	// MaxFile is the most bytes of one file's data: a config file the
	// engine sends, and a file one read gives. On an entries call it does not
	// bind: the codec's cap of the whole request is reached first.
	MaxFile = 1 << 20
	// MaxFiles is the most bytes of the files one read gives, in all. An
	// entries call carries the files of its reads in its request, so the
	// codec's cap of the whole request binds before it does.
	MaxFiles = 8 << 20
	// MaxReadFiles is the most files one read or glob gives.
	MaxReadFiles = 256
)
