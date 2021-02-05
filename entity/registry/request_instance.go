// add file in v.1.0.3
// instance_registry.go is file that save all instance of each request entity in map
// this registry is used to declare new instance with string in middleware.RequestValidator

package registry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"regexp"
)

var globalInstance = &requestInstance{}

type requestInstance map[string]interface{}

// register new request instance with parsing struct in file
func (ri *requestInstance) registerInstanceFromFile(path string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("unable to parse file, file: %s, err: %v\n", path, err.Error())
		return
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Obj == nil || x.Obj.Kind != ast.Typ {
				return true
			}
			switch s := x.Obj.Decl.(*ast.TypeSpec); s.Type.(type) {
			case *ast.StructType:
				ri.registerInstance(s.Name.Name)
			}
		}
		return true
	})
}

// register new request instance with instance name (only regex "^.Request$" key accept)
func (ri *requestInstance) registerInstance(instance string) {
	if !regexp.MustCompile("^.*Request$").MatchString(instance) {
		log.Fatalf("regex of all struct in request entity files must be \"^.*Request$\", struct name: %s", instance)
	}

	if sample, ok := requestSamples[instance]; !ok {
		log.Fatalf("register struct must be in request sample, struct name: %s\n", instance)
	} else {
		(*ri)[instance] = sample
	}
}
