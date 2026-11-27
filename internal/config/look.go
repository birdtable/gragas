package config

// Flags are the global flags that change how a run is drawn, read from the
// arguments before the terminal is built (the banner comes before the flags
// are parsed).
type Flags struct {
	NoColor, Accessible, Plain bool
}

// Look is what a terminal is drawn to, as the term adapter is told it.
type Look struct {
	// Columns is the width, 0 when the stream is not a terminal.
	Columns int
	// ColorBits is the colour depth, 0 for none.
	ColorBits int
	// Accessible is the accessible form.
	Accessible bool
}

// defaultColumns is the width of a terminal that did not say.
const defaultColumns = 80

// Look decides how a stream is drawn. A stream that is not a terminal has no
// width and no colour, unless CLICOLOR_FORCE or the settings ask for colour
// always. NO_COLOR, TERM=dumb, CI, --no-color, --plain, --accessible and the
// setting never all take the colour away; COLORTERM says how deep it goes.
// The kit reads no environment of the process, so this is the one place
// CLICOLOR_FORCE paints.
func (c Config) Look(tty bool, measured int, f Flags) Look {
	l := Look{Accessible: f.Accessible || c.Accessible}
	off := c.Colour() == "never" || c.NoColor || c.DumbTerm || c.CI || f.NoColor || f.Plain || l.Accessible
	forced := (c.Colour() == "always" || c.ForceColor) && !off
	if !tty && !forced {
		return l
	}
	l.Columns = c.Columns
	if l.Columns <= 0 {
		l.Columns = measured
	}
	if l.Columns <= 0 {
		l.Columns = defaultColumns
	}
	if off {
		return l
	}
	l.ColorBits = 8
	if c.TrueColor {
		l.ColorBits = 24
	}
	return l
}

// columnsOf is the COLUMNS variable as a width, 0 when it is not a plain
// number of digits.
func columnsOf(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' || n > 100000 {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
