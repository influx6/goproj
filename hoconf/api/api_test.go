package api

import (
	"fmt"
	"testing"
)

func checkError(t *testing.T, err error, msg string) {
	if err != nil {
		t.Fatalf("Message:%s Error: %s!", msg, err.Error())
		return
	}
	t.Logf("Message: %s NoError!", msg)
}

func TestJSONAPI(t *testing.T) {

	config := "./examples/honeyrc.json"

	api := NewConfig()

	err := api.LoadJSON(config)

	checkError(t, err, fmt.Sprintf("Loading JSON Config %s", config))

	t.Logf("API: %+s \n", api)
}

func TestYAMLAPI(t *testing.T) {

	config := "./examples/honeyrc.yaml"

	api := NewConfig()

	err := api.LoadYAML(config)

	checkError(t, err, fmt.Sprintf("Loading YAML Config %s", config))

	t.Logf("API: %+s \n", api)
}
