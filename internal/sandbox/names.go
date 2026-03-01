package sandbox

import (
	"fmt"
	"math/rand"
)

var adjectives = []string{
	"brave", "calm", "dark", "eager", "fancy",
	"great", "happy", "jolly", "kind", "lively",
	"merry", "nice", "proud", "quiet", "rapid",
	"sharp", "smart", "swift", "tough", "vivid",
	"witty", "zealous", "agile", "bold", "crisp",
}

var nouns = []string{
	"atlas", "beacon", "comet", "delta", "ember",
	"falcon", "glacier", "harbor", "iris", "jaguar",
	"kite", "lantern", "maple", "nebula", "orbit",
	"pixel", "quasar", "raven", "summit", "titan",
	"ultra", "vector", "wave", "xenon", "yacht",
}

func generateName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	return fmt.Sprintf("codex-%s-%s", adj, noun)
}
