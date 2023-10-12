package main

import (
	"log"

	e "github.com/cloudposse/atmos/internal/exec"
)

const imageName = "public.ecr.aws/r7v2l4o9/vpc:latest"
const dstDir = "/Users/andriyknysh/Documents/Projects/Go/src/github.com/cloudposse/atmos/tes/2"

func main() {
	err := e.ProcessOciImage(imageName, dstDir)
	if err != nil {
		log.Fatalf(err.Error())
	}
}
