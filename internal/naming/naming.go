package naming

import "fmt"

func Base(formula, version, tag string) string {
	return fmt.Sprintf("%s-%s.%s.bottle", formula, version, tag)
}

func BottleTarGz(formula, version, tag string) string {
	return Base(formula, version, tag) + ".tar.gz"
}

func BottleJSON(formula, version, tag string) string {
	return Base(formula, version, tag) + ".json"
}
