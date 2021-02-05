// add package in v.1.0.3
// registry package is used to get entity instance with string
// initializer.go is file that scan all struct about request entity in files in entity package

package registry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"regexp"
)

func init() {
	entityDir := "/usr/share/gateway/entity/"
	files, err := ioutil.ReadDir(entityDir)
	if err != nil {
		log.Fatalf("unable to read dir, dir: %s\n", entityDir)
	}
	var targetFiles []string
	for _, f := range files {
		if regexp.MustCompile("^request_.*.go$").MatchString(f.Name()) {
			targetFiles = append(targetFiles, entityDir + f.Name())
		}
	}

	for _, target := range targetFiles {
		globalInstance.registerInstanceFromFile(target)
	}
}
