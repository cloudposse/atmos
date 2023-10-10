// https://opencontainers.org/
// https://github.com/google/go-containerregistry

package exec

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	basicAuthn := &authn.Basic{
		Username: os.Getenv("DOCKER_USERNAME"),
		Password: os.Getenv("DOCKER_PASSWORD"),
	}

	withAuthOption := remote.WithAuth(basicAuthn)
	options := []remote.Option{withAuthOption}

	imageName := os.Args[1]

	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Fatalf("cannot parse reference of the image %s , detail: %v", imageName, err)
	}

	descriptor, err := remote.Get(ref, options...)
	if err != nil {
		log.Fatalf("cannot get image %s , detail: %v", imageName, err)
	}

	image, err := descriptor.Image()

	if err != nil {
		log.Fatalf("cannot convert image %s descriptor to v1.Image, detail: %v", imageName, err)
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		log.Fatalf("cannot extract config file of image %s, detail: %v", imageName, err)
	}

	prettyJSON, err := json.MarshalIndent(configFile, "", "    ")

	_, _ = io.Copy(os.Stdout, bytes.NewBuffer(prettyJSON))
}
