// add package in v.1.0.3
// registry package is used to get entity instance with string
// initializer.go is file that scan all struct about request entity in files in entity package

package registry

import (
	"github.com/dlclark/regexp2"
	"io/ioutil"
	"log"
)

func init() {
	entityDir := "/usr/share/gateway/entity/"
	files, err := ioutil.ReadDir(entityDir)
	if err != nil {
		log.Fatalf("unable to read dir, dir: %s\n", entityDir)
	}

	var targetFiles []string
	for _, f := range files {
		// use regexp2 to use (?!) expression
		if match, _ := regexp2.MustCompile("^request(?!_event).*.go$", regexp2.None).MatchString(f.Name()); match {
			targetFiles = append(targetFiles, entityDir + f.Name())
		}
	}

	for _, target := range targetFiles {
		globalInstance.registerInstanceFromFile(target)
	}

	// check if all request instance samples are registered in request instance registry
	for sample := range requestSamples {
		if _, ok := globalInstance.getInstance(sample); !ok {
			log.Fatalf("some reqeust sample didn't register in request entity registry, sample name: %s\n", sample)
		}
	}

	log.Println("Finished to initialize request entity instance in registry!!")
}
