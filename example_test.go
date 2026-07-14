package winmd_test

import (
	"fmt"
	"log"

	"github.com/deploymenttheory/go-winmd"
)

// Example opens a .winmd file, counts its COM interfaces, and decodes the
// first method signature — the core reader surface.
func Example() {
	file, err := winmd.Open("Windows.Win32.winmd")
	if err != nil {
		log.Fatal(err)
	}

	interfaces := 0
	for i := range file.Tables.TypeDefs {
		if file.Tables.TypeDefs[i].Flags&winmd.TypeAttrInterface != 0 {
			interfaces++
		}
	}
	fmt.Printf("%d types, %d COM interfaces\n", len(file.Tables.TypeDefs), interfaces)

	if len(file.Tables.Methods) > 0 {
		sig, err := file.MethodSignature(file.Tables.Methods[0].Signature)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("first method: %d params, return kind %d\n", len(sig.Params), sig.Return.Kind)
	}
}
