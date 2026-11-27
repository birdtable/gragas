package journeys

// standinCases returns the cases of the stand-in's bundle, by the folder each is
// kept in: one request and the response the stand-in answers it with, for each
// of the five operations its manifest provides. Each is a raw
// string, and so is spelled as the wire carries it.
func standinCases() map[string]string {
	return map[string]string{
		"1-entries/request.json":   `{"body":{"config":[{"data":{"text":"errcheck\n-gosec\nvet warning\n"},"path":"standin.conf"}],"have":[],"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"entries"}`,
		"1-entries/response.json":  `{"ok":{"entries":[{"key":"errcheck","on":true},{"key":"gosec","on":false},{"key":"vet","on":true,"severity":"warning"}]}}`,
		"2-entries/request.json":   `{"body":{"config":[],"have":[{"cut":false,"files":[],"exit":0,"kind":"run","round":0,"step":0,"stderr":{"text":""},"stdout":{"text":"errcheck\nvet warning\n"}}],"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"entries"}`,
		"2-entries/response.json":  `{"ok":{"entries":[{"key":"errcheck","on":true},{"key":"vet","on":true,"severity":"warning"}]}}`,
		"3-plan/request.json":      `{"body":{"config":[],"fixtures":[],"for":"run","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"plan"}`,
		"3-plan/response.json":     `{"ok":{"final":true,"steps":[{"argv":["--list"],"kind":"run","program":"standin"}]}}`,
		"4-plan/request.json":      `{"body":{"config":[],"fixtures":[],"for":"config","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"plan"}`,
		"4-plan/response.json":     `{"ok":{"final":false,"steps":[{"glob":"standin.conf","kind":"read"}]}}`,
		"5-plan/request.json":      `{"body":{"config":[{"data":{"text":"errcheck\n"},"path":"other.yml"}],"fixtures":[],"for":"config","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"plan"}`,
		"5-plan/response.json":     `{"error":{"code":"refused","message":"the standin reads standin.conf or standin.json, not other.yml","path":"/config/0/path"}}`,
		"6-findings/request.json":  `{"body":{"config":[],"fixtures":[],"have":[{"cut":false,"files":[],"exit":1,"kind":"run","round":0,"step":0,"stderr":{"text":""},"stdout":{"text":"a.go:3:errcheck:unchecked error\n"}}],"scratch":"/s","tool_version":"1.0.0"},"contract":1,"features":[],"op":"findings"}`,
		"6-findings/response.json": `{"ok":{"findings":[{"key":"errcheck","line":3,"message":"unchecked error","path":"a.go"}],"ran":true}}`,
		"7-findings/request.json":  `{"body":{"config":[],"fixtures":[],"have":[{"cut":false,"files":[],"exit":0,"kind":"run","round":0,"step":0,"stderr":{"text":""},"stdout":{"text":""}}],"scratch":"/s","tool_version":"1.0.0"},"contract":1,"features":[],"op":"findings"}`,
		"7-findings/response.json": `{"ok":{"findings":[],"ran":true}}`,
		"8-findings/request.json":  `{"body":{"config":[],"fixtures":[],"have":[],"scratch":"/s","tool_version":"1.0.0"},"contract":1,"features":[],"op":"findings"}`,
		"8-findings/response.json": `{"ok":{"findings":[],"ran":false,"why":"nothing ran"}}`,
		"9-rank/request.json":      `{"body":{"pairs":[{"aspect":"value","found":1,"key":"k","preset":2},{"aspect":"value","found":2,"key":"k","preset":1},{"aspect":"value","found":3,"key":"s","preset":3}],"tool_version":"1.0.0"},"contract":1,"features":[],"op":"rank"}`,
		"9-rank/response.json":     `{"ok":{"words":["tighter","weaker","same"]}}`,
		"10-preset/request.json":   `{"body":{"entries":[{"key":"errcheck","rules":["lint.errors.1"],"severity":"error","target":"source","trial":false}],"inputs":[],"mark":"gragas","tool_version":"1.0.0"},"contract":1,"features":[],"op":"preset"}`,
		"10-preset/response.json":  `{"ok":{"files":[{"path":"standin.conf","text":"# gragas\nerrcheck error\n"}]}}`,
		"11-preset/request.json":   `{"body":{"entries":[{"key":"errcheck","options":{"x":1},"rules":["lint.errors.1"],"target":"source","trial":false}],"inputs":[],"mark":"gragas","tool_version":"1.0.0"},"contract":1,"features":[],"op":"preset"}`,
		"11-preset/response.json":  `{"error":{"code":"refused","message":"the standin writes no options","path":"/entries/0/options"}}`,
		// A config refused by its content (gap 2), which plugin
		// test's C20 requires of every plugin.
		"12-plan/request.json":  `{"body":{"config":[{"data":{"text":"errcheck\nextends ../base.conf\n"},"path":"standin.conf"}],"fixtures":[],"for":"config","have":[],"round":0,"scratch":"/s","target":"source","tool_version":"1.0.0"},"contract":1,"features":[],"op":"plan"}`,
		"12-plan/response.json": `{"error":{"code":"refused","message":"the standin hands no tool this config: it sets extends, which reaches beyond its one file","path":"/config/0/data"}}`,
	}
}
