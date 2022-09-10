package plugin

import (
	"gopkg.in/yaml.v3"
	"os"
)

func LoadYaml(yamlFile string, conf any) error {
	bs, err := os.ReadFile(yamlFile)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(bs, conf)
	if err != nil {
		return err
	}
	return nil
}
