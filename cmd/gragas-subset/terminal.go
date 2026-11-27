package main

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// noTerminal is the Terminal port of a build that links no embedded file:
// host/term embeds the look's palette, so the subset does not import it. It
// draws nothing and every method that would answers port.ErrUnavailable
// (arch.ports.5, arch.ports.17, A61, A62).
type noTerminal struct{}

var _ port.Terminal = noTerminal{}

var errNoTerminal = fmt.Errorf("no terminal in the subset build: %w", port.ErrUnavailable)

func (noTerminal) Width() int  { return 0 }
func (noTerminal) Colors() int { return 0 }

func (noTerminal) Ask(context.Context, string) (string, error) { return "", errNoTerminal }
func (noTerminal) Say(string) error                            { return errNoTerminal }
func (noTerminal) Show(model.Screen) error                     { return errNoTerminal }
func (noTerminal) Banner(string) error                         { return errNoTerminal }
func (noTerminal) Line(context.Context) (string, error)        { return "", errNoTerminal }

func (noTerminal) CanAsk() bool { return false }
func (noTerminal) Pick(string, []model.Choice, string) (string, bool) {
	return "", false
}
