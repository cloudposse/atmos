package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type Semaphore struct {
	Wg sync.WaitGroup
	Ch chan int
}

// Limit on the number of simultaneously running goroutines.
// Depends on the number of processor cores, storage performance, amount of RAM, etc.
const grMax = 10

const imageName = "public.ecr.aws/r7v2l4o9/vpc:latest"
const tarFileName = "vpc.tar"
const dstDir = "2"

func extractTar(tarFileName string, dstDir string) error {
	f, err := os.Open(tarFileName)
	if err != nil {
		return err
	}

	sem := Semaphore{}
	sem.Ch = make(chan int, grMax)

	if err = untar(dstDir, f, &sem, true); err != nil {
		return err
	}

	fmt.Println("extractTar: wait for complete")
	sem.Wg.Wait()
	return nil
}

func untar(dst string, r io.Reader, sem *Semaphore, godeep bool) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {

		// if it's a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			if err := saveFile(tr, target, os.FileMode(header.Mode)); err != nil {
				return err
			}
			ext := filepath.Ext(target)

			// if it's tar file and we are on top level, extract it
			if (ext == ".tar" || ext == ".gz") && godeep {
				sem.Wg.Add(1)
				// A buffered channel is used to limit the number of simultaneously running goroutines
				sem.Ch <- 1
				f := strings.TrimSuffix(strings.TrimSuffix(header.Name, ".tar"), ".gz")
				// the file is unpacked to a directory with the file name (without extension)
				newDir := filepath.Join(dst, f)
				if err := os.Mkdir(newDir, 0755); err != nil {
					return err
				}
				go func(target string, newDir string, sem *Semaphore) {
					fmt.Println("start goroutine, chan length:", len(sem.Ch))
					fmt.Println("START:", target)
					defer sem.Wg.Done()
					defer func() { <-sem.Ch }()
					ft, err := os.Open(target)
					if err != nil {
						fmt.Println(err)
						return
					}
					defer ft.Close()
					if err := untar(newDir, ft, sem, true); err != nil {
						fmt.Println(err)
						return
					}
					fmt.Println("DONE:", target)
				}(target, newDir, sem)
			}
		}
	}
	return nil
}

func saveFile(r io.Reader, target string, mode os.FileMode) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return err
	}

	return nil
}

func main() {
	basicAuthn := &authn.Basic{
		Username: os.Getenv("DOCKER_USERNAME"),
		Password: os.Getenv("DOCKER_PASSWORD"),
	}

	withAuthOption := remote.WithAuth(basicAuthn)
	options := []remote.Option{withAuthOption}

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

	err = tarball.WriteToFile(tarFileName, ref, image)
	if err != nil {
		fmt.Println(err)
	}

	err = extractTar(tarFileName, dstDir)
	if err != nil {
		fmt.Println(err)
	}
}
