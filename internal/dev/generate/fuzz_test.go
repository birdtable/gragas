package generate_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/generate"
)

const consumerSeed = `{"consumer":"explorer","reads":[{"document":"bundle","fields":["rules[].id"]}]}`

// FuzzGenerateConsumer (tests.generated.1): a consumer's list of the fields it
// reads is a file a person writes, and the generator reads it as input. Whatever
// the bytes, Generate does not panic; it answers files or an error, and the
// error names the consumer file when the bytes are the cause.
func FuzzGenerateConsumer(f *testing.F) {
	for _, s := range []string{consumerSeed, `{}`, ``, `null`, `[]`, `{"consumer":"x","reads":null}`, `{"consumer":"x","reads":[{"document":"nothing","fields":["a.b"]}]}`,
		`{"consumer":"x","reads":[{"document":"bundle","fields":["","."," a","rules[]."]}]}`, consumerSeed[:len(consumerSeed)-1], `{"consumer":7}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, consumer []byte) {
		in := realInput(t)
		in.Consumers = map[string][]byte{"c.json": consumer}
		files, err := generate.Generate(in)
		if err != nil {
			return
		}
		if len(files) == 0 {
			t.Fatal("no error and no file")
		}
	})
}

// TestTheConsumerSeedReachesTheGenerator is the anti-vacuity check of
// FuzzGenerateConsumer: the seed generates, the seed with a field the schema
// lacks fails naming it, and the seed cut by one byte fails naming the file,
// so the target mutates what the generator reads.
func TestTheConsumerSeedReachesTheGenerator(t *testing.T) {
	in := realInput(t)
	in.Consumers = map[string][]byte{"c.json": []byte(consumerSeed)}
	if _, err := generate.Generate(in); err != nil {
		t.Fatalf("the seed: %v", err)
	}
	in.Consumers["c.json"] = []byte(strings.Replace(consumerSeed, "rules[].id", "rules[].nope", 1))
	if _, err := generate.Generate(in); err == nil || !strings.Contains(err.Error(), "rules[].nope") {
		t.Errorf("a field the schema lacks: %v", err)
	}
	in.Consumers["c.json"] = []byte(consumerSeed[:len(consumerSeed)-1])
	if _, err := generate.Generate(in); err == nil || !strings.Contains(err.Error(), "c.json") {
		t.Errorf("the seed cut: %v, want it to name c.json", err)
	}
}
