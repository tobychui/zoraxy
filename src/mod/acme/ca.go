package acme

/*
	CA.go

	This script load CA defination from embedded ca.json
*/
import (
	_ "embed"
	"encoding/json"
	"errors"
	"log"
)

// CA Defination, load from embeded json when startup
type CaDef struct {
	Production map[string]string
	Test       map[string]string
}

//go:embed ca.json
var caJson []byte

var caDef CaDef = CaDef{}

func init() {
	runtimeCaDef := CaDef{}
	err := json.Unmarshal(caJson, &runtimeCaDef)
	if err != nil {
		log.Println("[ERR] Unable to unmarshal CA def from embedded file. You sure your ca.json is valid?")
		return
	}

	caDef = runtimeCaDef
}

// Get the CA ACME server endpoint and error if not found
func loadCAApiServerFromName(caName string) (string, error) {
	val, ok := caDef.Production[caName]
	if !ok {
		return "", errors.New("This CA is not supported")
	}
	return val, nil
}
