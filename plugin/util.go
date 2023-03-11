package plugin

import (
	"gopkg.in/yaml.v3"
	"os"
	"strings"
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

// MatchTopic checks if a given topic matches a filter, accounting for filter
// wildcards. Eg. filter /a/b/+/c == topic a/b/d/c.
func MatchTopic(filter string, topic string) bool {
	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(topic, "/")

	for i := 0; i < len(filterParts); i++ {
		if i >= len(topicParts) {
			return false
		}

		if filterParts[i] == "+" {
			continue
		}

		if filterParts[i] == "#" {
			return true
		}

		if filterParts[i] != topicParts[i] {
			return false
		}
	}

	return true
}
