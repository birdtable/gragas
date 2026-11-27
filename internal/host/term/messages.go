package term

import "github.com/birdtable/go-console/ui"

// messages is every sentence the kit prints for gragas, in gragas's words:
// ui ships none of its own (go-console v0.5.0), so each session that draws or
// asks is given these. It began as go-console's internal English
// (internal/english/english.go at v0.6.0) and is gragas's to edit. The help of
// the five flags is the root's own (internal/cli/root/commands.go), though the
// kit registers none of them (FlagNames is empty): the root defines its own.
// The help command's words are cobra's, as the help goldens show them. Each
// format verb stays where the kit's English has it, in the same order.
func messages() ui.Messages {
	return ui.Messages{
		PlainHelp:      "Print plainly: nothing moves",
		AccessibleHelp: "Plain questions (ACCESSIBLE)",
		YesHelp:        "Answer yes to every question before a change",
		JSONHelp:       "Answer in one JSON document",
		NoColorHelp:    "No colour (NO_COLOR)",

		Yes: "Yes", No: "No",
		AnsweredYes: "yes", AnsweredNo: "no",
		AnsweredYesBy: "yes, by %s", AnsweredYesAdvance: "yes, in advance", AnsweredYesTyped: "yes, typed %s",
		YesFlagName:     "the yes flag",
		TypeNameTitle:   "Type %s to %s",
		TypeNameAdvance: "Or answer in advance with %s",
		TypeNameExactly: "type %s exactly, or press escape to leave it",
		NobodyToAsk:     "this would %s, and there is no terminal here to ask on",
		CouldNotAsk:     "this would %s, and the question could not be asked (%s)",

		CouldNotAskNothingDone: "%s, so nothing was done",
		FormNoTerminal:         "this needs answers, and there is no terminal here to ask on",
		FormNotAnswered:        "nothing was answered",
		FormCouldNot:           "the questions could not be asked (%s)",

		DryWould: "would %s",

		LiveNothingChanged:    "nothing was changed",
		LiveNotTheName:        "that is not the name, so nothing was changed",
		LiveNothingSelected:   "nothing is selected",
		LiveNothingToStepInto: "nothing to step into",
		LiveNoName:            "this row has no name to type, so nothing was changed",
		LiveBack:              "back from %s",

		HelpArgs: "[topic or command]", HelpShort: "Help about a command, or a lesson on a topic", HelpUnknownTopic: "Unknown help topic %#q",
		NameOnCommandLine: "Or name it on the command line: %s", AnswerWith: "Or answer with %s",
		MenuCouldNotShow: "could not show the menu (%s), so here is the help instead", CouldNotAskFor: "could not ask for %s (%s)", MenuNothingPicked: "nothing was picked",
		InternalError:   "the command stopped on an internal error",
		FollowFollowing: "following", FollowPaused: "paused, end follows", FollowKeys: " · ↑/↓ scroll · ctrl-c leave", PageKeys: "%3.0f%% · ↑/↓ scroll · q leave",
		HelpFlagHelp: "help for %s", HelpCommandArgs: "[command]", HelpAnyShort: "Help about any command", VersionLine: "version %s",
	}
}
